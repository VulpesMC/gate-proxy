package client

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vulpesmc/gate-proxy/plugins/vulpescloud/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func BuildClientTLS(nodeCertPEM, nodeKeyPEM, caCertPEM []byte) (credentials.TransportCredentials, error) {
	// Handles PKCS#1, PKCS#8 and EC private keys automatically
	cert, err := tls.X509KeyPair(nodeCertPEM, nodeKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load node keypair: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert}, // client cert for mTLS
		RootCAs:      pool,                    // trust only your CA
		MinVersion:   tls.VersionTLS12,
	}), nil
}

func Connect() (*grpc.ClientConn, error) {
	certDir := filepath.Join("vulpescloud", "certs")
	nodeCertPath := filepath.Join(certDir, "node.crt")
	nodeKeyPath := filepath.Join(certDir, "node.key")
	caCertPath := filepath.Join(certDir, "ca.crt")

	nodeCert, err := os.ReadFile(nodeCertPath)
	if err != nil {
		return nil, err
	}
	nodeKey, err := os.ReadFile(nodeKeyPath)
	if err != nil {
		return nil, err
	}
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}

	creds, err := BuildClientTLS(nodeCert, nodeKey, caCert)
	if err != nil {
		return nil, err
	}
	var opts []grpc.DialOption
	opts = append(opts,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(auth.AuthInterceptor(os.Getenv("secret"))),
		grpc.WithStreamInterceptor(auth.AuthStreamInterceptor(os.Getenv("secret"))),
	)

	client, err := grpc.NewClient(
		fmt.Sprintf("%s:%s", os.Getenv("grpc_hostname"), os.Getenv("grpc_port")),
		opts...,
	)

	return client, err
}
