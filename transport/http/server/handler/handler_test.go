// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

func TestK8sHealtz(t *testing.T) {
	w := writer.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)

	rw := writer.NewReadWriter(rec, req, w)
	K8sHealtz(rw)
	rw.Release()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp struct {
		Data HealthResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if resp.Data.Status != "ok" {
		t.Fatalf("status = %q", resp.Data.Status)
	}
}

func TestK8sReadyz(t *testing.T) {
	w := writer.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)

	rw := writer.NewReadWriter(rec, req, w)
	K8sReadyz(rw)
	rw.Release()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp struct {
		Data HealthResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if resp.Data.Status != "ok" {
		t.Fatalf("status = %q", resp.Data.Status)
	}
}

func TestPing(t *testing.T) {
	w := writer.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping", nil)

	rw := writer.NewReadWriter(rec, req, w)
	Ping(rw)
	rw.Release()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp struct {
		Data PingResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if resp.Data.Message != "pong" {
		t.Fatalf("message = %q", resp.Data.Message)
	}
}

func TestHealthResponse_JSON(t *testing.T) {
	resp := HealthResponse{Status: "ok"}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var result map[string]string
	json.Unmarshal(b, &result)
	if result["status"] != "ok" {
		t.Fatalf("status = %q", result["status"])
	}
}

func TestPingResponse_JSON(t *testing.T) {
	resp := PingResponse{Message: "pong"}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var result map[string]string
	json.Unmarshal(b, &result)
	if result["message"] != "pong" {
		t.Fatalf("message = %q", result["message"])
	}
}
