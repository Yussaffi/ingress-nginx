/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package proxyssl

import (
	"testing"

	api "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/ingress-nginx/internal/ingress/annotations/parser"
	"k8s.io/ingress-nginx/internal/ingress/errors"
	"k8s.io/ingress-nginx/internal/ingress/resolver"
)

const (
	defaultDemoSecret = "default/demo-secret"
	proxySslCiphers   = "HIGH:-SHA"
	off               = "off"
	sslServerName     = "w00t"
	defaultProtocol   = "TLSv1.2 TLSv1.3"
)

func buildIngress() *networking.Ingress {
	defaultBackend := networking.IngressBackend{
		Service: &networking.IngressServiceBackend{
			Name: "default-backend",
			Port: networking.ServiceBackendPort{
				Number: 80,
			},
		},
	}

	return &networking.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: networking.IngressSpec{
			DefaultBackend: &networking.IngressBackend{
				Service: &networking.IngressServiceBackend{
					Name: "default-backend",
					Port: networking.ServiceBackendPort{
						Number: 80,
					},
				},
			},
			Rules: []networking.IngressRule{
				{
					Host: "foo.bar.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:    "/foo",
									Backend: defaultBackend,
								},
							},
						},
					},
				},
			},
		},
	}
}

// mocks the resolver for proxySSL
type mockSecret struct {
	resolver.Mock
}

// GetAuthCertificate from mockSecret mocks the GetAuthCertificate for backend certificate authentication
func (m mockSecret) GetAuthCertificate(name string) (*resolver.AuthSSLCert, error) {
	if name != defaultDemoSecret {
		return nil, errors.Errorf("there is no secret with name %v", name)
	}

	return &resolver.AuthSSLCert{
		Secret:     defaultDemoSecret,
		CAFileName: "/ssl/ca.crt",
		CASHA:      "abc",
	}, nil
}

func TestAnnotations(t *testing.T) {
	ing := buildIngress()
	data := map[string]string{}

	data[parser.GetAnnotationWithPrefix(proxySSLSecretAnnotation)] = defaultDemoSecret
	data[parser.GetAnnotationWithPrefix("proxy-ssl-ciphers")] = proxySslCiphers
	data[parser.GetAnnotationWithPrefix("proxy-ssl-name")] = "$host"
	data[parser.GetAnnotationWithPrefix("proxy-ssl-protocols")] = "TLSv1.3 TLSv1.2"
	data[parser.GetAnnotationWithPrefix("proxy-ssl-server-name")] = "on"
	data[parser.GetAnnotationWithPrefix("proxy-ssl-session-reuse")] = off
	data[parser.GetAnnotationWithPrefix("proxy-ssl-verify")] = "on"
	data[parser.GetAnnotationWithPrefix("proxy-ssl-verify-depth")] = "3"

	ing.SetAnnotations(data)

	fakeSecret := &mockSecret{}
	i, err := NewParser(fakeSecret).Parse(ing)
	if err != nil {
		t.Errorf("Unexpected error with ingress: %v", err)
	}

	u, ok := i.(*Config)
	if !ok {
		t.Errorf("expected *Config but got %v", u)
	}

	secret, err := fakeSecret.GetAuthCertificate(defaultDemoSecret)
	if err != nil {
		t.Errorf("unexpected error getting secret %v", err)
	}

	if u.AuthSSLCert.Secret != secret.Secret {
		t.Errorf("expected %v but got %v", secret.Secret, u.AuthSSLCert.Secret)
	}
	if u.Ciphers != proxySslCiphers {
		t.Errorf("expected %v but got %v", proxySslCiphers, u.Ciphers)
	}
	if u.Protocols != defaultProtocol {
		t.Errorf("expected %v but got %v", defaultProtocol, u.Protocols)
	}
	if u.Verify != "on" {
		t.Errorf("expected %v but got %v", "on", u.Verify)
	}
	if u.VerifyDepth != 3 {
		t.Errorf("expected %v but got %v", 3, u.VerifyDepth)
	}
	if u.ProxySSLName != "$host" {
		t.Errorf("expected %v but got %v", "$host", u.ProxySSLName)
	}
	if u.ProxySSLServerName != "on" {
		t.Errorf("expected %v but got %v", "on", u.ProxySSLServerName)
	}
}

func TestInvalidAnnotations(t *testing.T) {
	ing := buildIngress()
	fakeSecret := &mockSecret{}
	data := map[string]string{}

	// No annotation
	_, err := NewParser(fakeSecret).Parse(ing)
	if err == nil {
		t.Errorf("Expected error with ingress but got nil")
	}

	// Invalid NameSpace
	data[parser.GetAnnotationWithPrefix("proxy-ssl-secret")] = "demo-secret"
	ing.SetAnnotations(data)
	_, err = NewParser(fakeSecret).Parse(ing)
	if err == nil {
		t.Errorf("Expected error with ingress but got nil")
	}

	// Invalid Proxy Certificate
	data[parser.GetAnnotationWithPrefix("proxy-ssl-secret")] = "default/invalid-demo-secret"
	ing.SetAnnotations(data)
	_, err = NewParser(fakeSecret).Parse(ing)
	if err == nil {
		t.Errorf("Expected error with ingress but got nil")
	}

	// Invalid optional Annotations
	data[parser.GetAnnotationWithPrefix("proxy-ssl-secret")] = defaultDemoSecret
	data[parser.GetAnnotationWithPrefix("proxy-ssl-protocols")] = "TLSv111 SSLv1"
	data[parser.GetAnnotationWithPrefix("proxy-ssl-server-name")] = sslServerName
	data[parser.GetAnnotationWithPrefix("proxy-ssl-session-reuse")] = sslServerName
	data[parser.GetAnnotationWithPrefix("proxy-ssl-verify")] = sslServerName
	data[parser.GetAnnotationWithPrefix("proxy-ssl-verify-depth")] = "abcd"
	ing.SetAnnotations(data)

	i, err := NewParser(fakeSecret).Parse(ing)
	if err != nil {
		t.Errorf("Unexpected error with ingress: %v", err)
	}
	u, ok := i.(*Config)
	if !ok {
		t.Errorf("expected *Config but got %v", u)
	}

	if u.Protocols != defaultProxySSLProtocols {
		t.Errorf("expected %v but got %v", defaultProxySSLProtocols, u.Protocols)
	}
	if u.Verify != defaultProxySSLVerify {
		t.Errorf("expected %v but got %v", defaultProxySSLVerify, u.Verify)
	}
	if u.VerifyDepth != defaultProxySSLVerifyDepth {
		t.Errorf("expected %v but got %v", defaultProxySSLVerifyDepth, u.VerifyDepth)
	}
	if u.ProxySSLServerName != defaultProxySSLServerName {
		t.Errorf("expected %v but got %v", defaultProxySSLServerName, u.ProxySSLServerName)
	}
}

func TestEquals(t *testing.T) {
	cfg1 := &Config{}
	cfg2 := &Config{}

	// compare nil
	result := cfg1.Equal(nil)
	if result != false {
		t.Errorf("Expected false")
	}

	// Different Certs
	sslCert1 := resolver.AuthSSLCert{
		Secret:     defaultDemoSecret,
		CAFileName: "/ssl/ca.crt",
		CASHA:      "abc",
	}
	sslCert2 := resolver.AuthSSLCert{
		Secret:     "default/other-demo-secret",
		CAFileName: "/ssl/ca.crt",
		CASHA:      "abc",
	}
	cfg1.AuthSSLCert = sslCert1
	cfg2.AuthSSLCert = sslCert2
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.AuthSSLCert = sslCert1

	// Different Ciphers
	cfg1.Ciphers = "DEFAULT"
	cfg2.Ciphers = proxySslCiphers
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.Ciphers = "DEFAULT"

	// Different Protocols
	cfg1.Protocols = defaultProtocol
	cfg2.Protocols = "SSLv3 TLSv1 TLSv1.2 TLSv1.3"
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.Protocols = defaultProtocol

	// Different Verify
	cfg1.Verify = off
	cfg2.Verify = "on"
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.Verify = off

	// Different VerifyDepth
	cfg1.VerifyDepth = 1
	cfg2.VerifyDepth = 2
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.VerifyDepth = 1

	// Different ProxySSLServerName
	cfg1.ProxySSLServerName = off
	cfg2.ProxySSLServerName = "on"
	result = cfg1.Equal(cfg2)
	if result != false {
		t.Errorf("Expected false")
	}
	cfg2.ProxySSLServerName = off

	// Equal Configs
	result = cfg1.Equal(cfg2)
	if result != true {
		t.Errorf("Expected true")
	}
}
