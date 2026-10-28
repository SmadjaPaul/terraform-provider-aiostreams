package aiostreams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectNonHTTPS(t *testing.T) {
	for _, u := range []string{"http://10.0.0.1:3000", "https://user:pass@host", "https://host/?token=secret"} {
		if _, err := New(u); err == nil {
			t.Errorf("accepted %s", u)
		}
	}
}
func TestAllowLoopbackHTTP(t *testing.T) {
	for _, u := range []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://[::1]:3000"} {
		if _, err := New(u); err != nil {
			t.Errorf("rejected %s: %v", u, err)
		}
	}
}
func TestLifecycle(t *testing.T) {
	var saved json.RawMessage
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "POST" {
			u, p, ok := r.BasicAuth()
			if !ok || u != "test-uuid" || p != "pass" {
				w.WriteHeader(401)
				w.Write([]byte(`{"success":false,"detail":"never expose secret"}`))
				return
			}
		}
		switch r.Method {
		case "POST":
			var b struct {
				Config   json.RawMessage `json:"config"`
				Password string          `json:"password"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			saved = b.Config
			w.WriteHeader(201)
			w.Write([]byte(`{"success":true,"data":{"uuid":"test-uuid","encryptedPassword":"secret"}}`))
		case "GET":
			w.Write([]byte(`{"success":true,"data":{"userData":` + string(saved) + `,"encryptedPassword":"secret"}}`))
		case "PUT":
			var b struct {
				Config json.RawMessage `json:"config"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			saved = b.Config
			w.Write([]byte(`{"success":true,"data":{"uuid":"test-uuid"}}`))
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	c.HTTP = srv.Client()
	ctx := context.Background()
	id, e := c.Create(ctx, "pass", json.RawMessage(`{"addons":[]}`))
	if e != nil || id.UUID != "test-uuid" {
		t.Fatalf("create: %+v %v", id, e)
	}
	cred := Credentials{UUID: id.UUID, Password: "pass"}
	if e = c.Update(ctx, cred, json.RawMessage(`{"addons":["cinemeta"]}`)); e != nil {
		t.Fatal(e)
	}
	got, e := c.Get(ctx, cred)
	if e != nil || !strings.Contains(string(got), "cinemeta") {
		t.Fatalf("get: %s %v", got, e)
	}
}
func TestSuppressServerDetails(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); w.Write([]byte("token=private")) }))
	defer s.Close()
	c, _ := New(s.URL)
	c.HTTP = s.Client()
	_, err := c.Get(context.Background(), Credentials{"x", "y"})
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatalf("leaked response: %v", err)
	}
}

func TestOperatorSessionAuthorizesProtectedCreate(t *testing.T) {
	var saved json.RawMessage
	loginCalls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			loginCalls++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["username"] != "operator" || body["password"] != "operator-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "aiostreams.session", Value: "session", Path: "/"})
			w.Write([]byte(`{"success":true,"data":{"username":"operator"}}`))
		case "/api/v1/user":
			cookie, err := r.Cookie("aiostreams.session")
			if err != nil || cookie.Value != "session" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if r.Method == http.MethodPost {
				var body struct {
					Config json.RawMessage `json:"config"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				saved = body.Config
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"success":true,"data":{"uuid":"test-uuid"}}`))
				return
			}
			w.Write([]byte(`{"success":true,"data":{"userData":` + string(saved) + `}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP = srv.Client()
	c.HTTP.Jar = jar
	if err := c.SetOperatorAuth("operator", "operator-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(context.Background(), "config-secret", json.RawMessage(`{"addons":[]}`)); err != nil {
		t.Fatal(err)
	}
	if loginCalls != 1 {
		t.Fatalf("expected one operator login, got %d", loginCalls)
	}
}
