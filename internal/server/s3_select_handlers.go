package server

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Kyaxris-Labs/Noctaxris/internal/catalog"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authn"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

type s3SelectObjectContentRequestXML struct {
	XMLName            xml.Name                     `xml:"SelectObjectContentRequest"`
	Expression         string                       `xml:"Expression"`
	ExpressionType     string                       `xml:"ExpressionType"`
	InputSerialization s3SelectInputSerializationXML `xml:"InputSerialization"`
	OutputSerialization s3SelectOutputSerializationXML `xml:"OutputSerialization"`
}

// AWS examples sometimes use SelectRequest as the root element.
type s3SelectRequestAltXML struct {
	XMLName             xml.Name                      `xml:"SelectRequest"`
	Expression          string                        `xml:"Expression"`
	ExpressionType      string                        `xml:"ExpressionType"`
	InputSerialization  s3SelectInputSerializationXML `xml:"InputSerialization"`
	OutputSerialization s3SelectOutputSerializationXML `xml:"OutputSerialization"`
}

type s3SelectInputSerializationXML struct {
	CompressionType string              `xml:"CompressionType"`
	CSV             *s3SelectCSVInputXML  `xml:"CSV"`
	JSON            *s3SelectJSONInputXML `xml:"JSON"`
	Parquet         *struct{}           `xml:"Parquet"`
}

type s3SelectCSVInputXML struct {
	FileHeaderInfo  string `xml:"FileHeaderInfo"`
	FieldDelimiter  string `xml:"FieldDelimiter"`
	RecordDelimiter string `xml:"RecordDelimiter"`
}

type s3SelectJSONInputXML struct {
	Type string `xml:"Type"`
}

type s3SelectOutputSerializationXML struct {
	CSV  *s3SelectCSVOutputXML  `xml:"CSV"`
	JSON *s3SelectJSONOutputXML `xml:"JSON"`
}

type s3SelectCSVOutputXML struct {
	FieldDelimiter  string `xml:"FieldDelimiter"`
	RecordDelimiter string `xml:"RecordDelimiter"`
}

type s3SelectJSONOutputXML struct {
	RecordDelimiter string `xml:"RecordDelimiter"`
}

type s3SelectSimplifiedResponse struct {
	Records []json.RawMessage `json:"Records"`
	Stats   s3SelectStatsJSON `json:"Stats"`
	End     bool              `json:"End"`
}

type s3SelectStatsJSON struct {
	BytesScanned   int64 `json:"BytesScanned"`
	BytesProcessed int64 `json:"BytesProcessed"`
	BytesReturned  int64 `json:"BytesReturned"`
}

func firstRuneOr(s string, def rune) rune {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return def
	}
	return r
}

func parseSelectRequestXML(body []byte) (s3SelectObjectContentRequestXML, error) {
	var req s3SelectObjectContentRequestXML
	if err := xml.Unmarshal(body, &req); err == nil && strings.TrimSpace(req.Expression) != "" {
		return req, nil
	}
	var alt s3SelectRequestAltXML
	if err := xml.Unmarshal(body, &alt); err != nil {
		return s3SelectObjectContentRequestXML{}, err
	}
	return s3SelectObjectContentRequestXML{
		Expression:          alt.Expression,
		ExpressionType:      alt.ExpressionType,
		InputSerialization:  alt.InputSerialization,
		OutputSerialization: alt.OutputSerialization,
	}, nil
}

func selectRequestToStore(req s3SelectObjectContentRequestXML) (store.S3SelectRequest, error) {
	in := store.S3SelectInput{Compression: req.InputSerialization.CompressionType}
	switch {
	case req.InputSerialization.Parquet != nil:
		return store.S3SelectRequest{}, store.ErrSelectUnsupportedFormat
	case req.InputSerialization.CSV != nil:
		in.Format = "CSV"
		in.FileHeaderInfo = req.InputSerialization.CSV.FileHeaderInfo
		in.FieldDelimiter = firstRuneOr(req.InputSerialization.CSV.FieldDelimiter, ',')
		in.RecordDelimiter = req.InputSerialization.CSV.RecordDelimiter
	case req.InputSerialization.JSON != nil:
		in.Format = "JSON"
		in.JSONType = req.InputSerialization.JSON.Type
	default:
		return store.S3SelectRequest{}, store.ErrSelectUnsupportedFormat
	}
	out := store.S3SelectOutput{}
	switch {
	case req.OutputSerialization.CSV != nil:
		out.Format = "CSV"
		out.FieldDelimiter = firstRuneOr(req.OutputSerialization.CSV.FieldDelimiter, ',')
		out.RecordDelimiter = req.OutputSerialization.CSV.RecordDelimiter
	case req.OutputSerialization.JSON != nil:
		out.Format = "JSON"
		out.RecordDelimiter = req.OutputSerialization.JSON.RecordDelimiter
	default:
		out.Format = in.Format
	}
	return store.S3SelectRequest{
		Expression:     req.Expression,
		ExpressionType: req.ExpressionType,
		Input:          in,
		Output:         out,
	}, nil
}

func (s *Server) s3SelectObjectContent(w http.ResponseWriter, r *http.Request, body []byte, requestID, eventID string, verified *authn.Verified, readOnly bool, bucket, key string) {
	ref, ok := s.s3RequireBucket(w, r, requestID, eventID, verified, readOnly, bucket, "SelectObjectContent")
	if !ok {
		return
	}
	resource := store.ObjectARN(bucket, strings.TrimPrefix(key, "/"))
	if !s.authorizeS3(verified, catalog.ActionS3SelectObjectContent, resource, ref.policy, ref.accountID) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusForbidden, "AccessDenied",
			"Access Denied", "SelectObjectContent")
		return
	}
	reqXML, err := parseSelectRequestXML(body)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "MalformedXML",
			"The XML you provided was not well-formed or did not validate against our published schema.", "SelectObjectContent")
		return
	}
	selectReq, err := selectRequestToStore(reqXML)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
			"Unsupported SelectObjectContent input or output serialization.", "SelectObjectContent")
		return
	}

	meta, _, data, err := s.store.GetObjectVersion(ref.accountID, bucket, key, "")
	if errors.Is(err, store.ErrNoSuchKey) || errors.Is(err, store.ErrInvalidObjectKey) {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusNotFound, "NoSuchKey",
			"The specified key does not exist.", "SelectObjectContent")
		return
	}
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "SelectObjectContent")
		return
	}
	plain, err := s.decryptObjectPayload(verified, meta, data)
	if err != nil {
		code, msg := "InternalError", "Internal error"
		status := http.StatusInternalServerError
		if errors.Is(err, errS3AccessDenied) {
			code, msg, status = "AccessDenied", "Access Denied", http.StatusForbidden
		}
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, status, code, msg, "SelectObjectContent")
		return
	}
	if int64(len(plain)) > store.MaxSelectObjectBytes {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
			"SelectObjectContent supports objects up to "+strconv.Itoa(store.MaxSelectObjectBytes)+" bytes in this lab.", "SelectObjectContent")
		return
	}

	result, err := store.RunS3Select(plain, selectReq)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSelectUnsupportedSQL):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				err.Error(), "SelectObjectContent")
		case errors.Is(err, store.ErrSelectUnsupportedFormat):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				err.Error(), "SelectObjectContent")
		case errors.Is(err, store.ErrSelectObjectTooLarge):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				"SelectObjectContent supports objects up to "+strconv.Itoa(store.MaxSelectObjectBytes)+" bytes in this lab.", "SelectObjectContent")
		case errors.Is(err, store.ErrSelectInvalidRequest):
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusBadRequest, "InvalidRequest",
				err.Error(), "SelectObjectContent")
		default:
			s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
				"Internal error", "SelectObjectContent")
		}
		return
	}

	resp := s3SelectSimplifiedResponse{
		Records: result.Records,
		Stats: s3SelectStatsJSON{
			BytesScanned:   result.BytesScanned,
			BytesProcessed: result.BytesProcessed,
			BytesReturned:  result.BytesReturned,
		},
		End: true,
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		s.writeS3Error(w, r, requestID, eventID, verified, readOnly, http.StatusInternalServerError, "InternalError",
			"Internal error", "SelectObjectContent")
		return
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
	s.writeSuccessAudit(r, requestID, eventID, verified, "s3.amazonaws.com", "SelectObjectContent", readOnly)
}
