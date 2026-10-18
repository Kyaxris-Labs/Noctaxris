// Package awschunked decodes Amazon S3 aws-chunked request bodies
// (Content-Encoding: aws-chunked) used by AWS CLI/SDK streaming uploads
// and checksum trailers.
package awschunked

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Decode reads an aws-chunked body and returns the decoded object payload and
// any trailing headers (for example x-amz-checksum-crc64nvme). Chunk-signature
// extensions are accepted and ignored (lab does not re-verify per-chunk SigV4).
func Decode(r io.Reader) (payload []byte, trailers http.Header, err error) {
	br := bufio.NewReader(r)
	var out bytes.Buffer
	trailers = make(http.Header)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, nil, fmt.Errorf("aws-chunked header: %w", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		sizePart, _, _ := strings.Cut(line, ";")
		sizePart = strings.TrimSpace(sizePart)
		if sizePart == "" {
			return nil, nil, fmt.Errorf("aws-chunked: empty chunk size line")
		}
		size, err := strconv.ParseInt(sizePart, 16, 64)
		if err != nil || size < 0 {
			return nil, nil, fmt.Errorf("aws-chunked: invalid chunk size %q", sizePart)
		}
		if size == 0 {
			if err := readTrailers(br, trailers); err != nil {
				return nil, nil, err
			}
			return out.Bytes(), trailers, nil
		}
		if _, err := io.CopyN(&out, br, size); err != nil {
			return nil, nil, fmt.Errorf("aws-chunked data: %w", err)
		}
		// Chunk data is followed by CRLF.
		crlf := make([]byte, 2)
		if _, err := io.ReadFull(br, crlf); err != nil {
			return nil, nil, fmt.Errorf("aws-chunked crlf: %w", err)
		}
		if crlf[0] != '\r' || crlf[1] != '\n' {
			return nil, nil, fmt.Errorf("aws-chunked: missing chunk CRLF")
		}
	}
}

func readTrailers(br *bufio.Reader, trailers http.Header) error {
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("aws-chunked trailer: %w", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			return nil
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("aws-chunked: malformed trailer %q", line)
		}
		trailers.Add(http.CanonicalHeaderKey(strings.TrimSpace(key)), strings.TrimSpace(val))
	}
}

// HasEncoding reports whether Content-Encoding lists aws-chunked.
func HasEncoding(contentEncoding string) bool {
	for _, part := range strings.Split(contentEncoding, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "aws-chunked") {
			return true
		}
	}
	return false
}

// StripEncoding removes aws-chunked tokens from a Content-Encoding header value.
func StripEncoding(contentEncoding string) string {
	parts := strings.Split(contentEncoding, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" || strings.EqualFold(p, "aws-chunked") {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}
