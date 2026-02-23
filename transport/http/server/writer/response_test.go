// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"encoding/json"
	"testing"
)

func TestResponse_JSON_Data(t *testing.T) {
	resp := Response{Data: "hello"}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if result["data"] != "hello" {
		t.Fatalf("data = %v", result["data"])
	}
	if _, ok := result["error"]; ok {
		t.Fatal("error should be omitted")
	}
}

func TestResponse_JSON_Error(t *testing.T) {
	resp := Response{Error: &Error{Code: "NOT_FOUND", Message: "not found"}}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if _, ok := result["data"]; ok {
		t.Fatal("data should be omitted")
	}
	errMap := result["error"].(map[string]any)
	if errMap["code"] != "NOT_FOUND" {
		t.Fatalf("code = %v", errMap["code"])
	}
}

func TestResponse_JSON_Empty(t *testing.T) {
	resp := Response{}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	// Both data and error should be omitted
	var result map[string]any
	json.Unmarshal(b, &result)
	if len(result) != 0 {
		t.Fatalf("empty response should have no fields, got %v", result)
	}
}

func TestError_JSON(t *testing.T) {
	e := Error{Code: "TEST", Message: "msg"}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var result map[string]any
	json.Unmarshal(b, &result)
	if result["code"] != "TEST" {
		t.Fatalf("code = %v", result["code"])
	}
	if result["message"] != "msg" {
		t.Fatalf("message = %v", result["message"])
	}
}
