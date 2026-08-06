package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers/builder"
)

func sftpTestProfile() *operatorv1.GameProfile {
	return &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "minecraft"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName:  "Minecraft",
			Capabilities: &operatorv1.CapabilitiesSpec{SFTP: true},
		},
	}
}

func sftpTestServer() *operatorv1.GameServer {
	return &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
		Status: operatorv1.GameServerStatus{
			Endpoints: []operatorv1.Endpoint{
				{Name: "game", Address: "gs1.games.example.com", Port: 25565},
				{Name: builder.SFTPPortName, Address: "gs1.games.example.com", Port: builder.SFTPPort},
			},
		},
	}
}

func sftpTestSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: builder.SFTPSecretName("gs1"), Namespace: "default"},
		Data: map[string][]byte{
			builder.SFTPUsernameKey: []byte(builder.SFTPUsername),
			builder.SFTPPasswordKey: []byte("0123456789abcdef0123456789abcdef"),
		},
	}
}

func TestGetSFTPInfo_Success(t *testing.T) {
	api, _ := setupTestAPI(sftpTestProfile(), sftpTestServer(), sftpTestSecret())
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/sftp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var info struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if info.Host != "gs1.games.example.com" {
		t.Fatalf("unexpected host %q", info.Host)
	}
	if info.Port != builder.SFTPPort {
		t.Fatalf("unexpected port %d", info.Port)
	}
	if info.Username != builder.SFTPUsername || info.Password != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected credentials %q/%q", info.Username, info.Password)
	}
}

func TestGetSFTPInfo_ServerNotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/missing/sftp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetSFTPInfo_CapabilityDisabled(t *testing.T) {
	profile := sftpTestProfile()
	profile.Spec.Capabilities = &operatorv1.CapabilitiesSpec{SFTP: false}
	api, _ := setupTestAPI(profile, sftpTestServer(), sftpTestSecret())
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/sftp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetSFTPInfo_EndpointNotPublished(t *testing.T) {
	server := sftpTestServer()
	server.Status.Endpoints = nil
	api, _ := setupTestAPI(sftpTestProfile(), server, sftpTestSecret())
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/sftp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetSFTPInfo_SecretMissing(t *testing.T) {
	api, _ := setupTestAPI(sftpTestProfile(), sftpTestServer())
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/sftp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
