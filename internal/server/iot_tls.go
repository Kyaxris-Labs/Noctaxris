package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

// DeviceClientAuthTLSConfig requests a client certificate (VerifyClientCertIfGiven)
// and verifies it against the lab IoT CA when presented. Python and curl --cert
// send the device cert only when ClientAuth is set.
func DeviceClientAuthTLSConfig(serverCert tls.Certificate, caPEM string) *tls.Config {
	pool := x509.NewCertPool()
	if strings.TrimSpace(caPEM) != "" {
		pool.AppendCertsFromPEM([]byte(caPEM))
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    pool,
	}
}

func tlsConfigFromPEMFiles(certFile, keyFile, caPEM string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("iot tls: load server certificate: %w", err)
	}
	return DeviceClientAuthTLSConfig(cert, caPEM), nil
}

func (s *Server) iotDeviceListenerTLSConfig() (*tls.Config, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("iot tls: store required")
	}
	caPEM, err := s.store.LabIoTCACertificatePEM()
	if err != nil {
		return nil, err
	}
	dns, ips := store.DefaultIoTServerSANs()
	host := strings.TrimSpace(s.cfg.IoTEndpointHost)
	if host != "" && net.ParseIP(host) == nil && !strings.EqualFold(host, "localhost") {
		dns = append(dns,
			"data.iot."+host,
			"data-ats.iot."+host,
			"jobs.iot."+host,
			"credentials.iot."+host,
			host,
		)
	}
	srv, err := s.store.EnsureLabIoTServerCertificate(dns, ips)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair([]byte(srv.CertPEM), []byte(srv.KeyPEM))
	if err != nil {
		return nil, fmt.Errorf("iot tls: parse server certificate: %w", err)
	}
	return DeviceClientAuthTLSConfig(cert, caPEM), nil
}

func (s *Server) iotDeviceHTTPPort() string {
	listen := strings.TrimSpace(s.cfg.IoTTLSListen)
	if listen == "" {
		return iotLabEndpointPort
	}
	_, port, err := net.SplitHostPort(listen)
	if err == nil && port != "" && port != "0" {
		return port
	}
	return "8443"
}
