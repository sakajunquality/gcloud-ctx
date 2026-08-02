package gcloud

import "testing"

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"default", false},
		{"my-context", false},
		{"a", false},
		{"a1", false},
		{"a-1-b-2", false},
		{"", true},
		{"NONE", true},
		{"None", true},
		{"1abc", true},
		{"-abc", true},
		{"Abc", true},
		{"abc_def", true},
		{"abc def", true},
		{"abc.def", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestIsNone(t *testing.T) {
	if !IsNone("NONE") {
		t.Error("IsNone(\"NONE\") = false, want true")
	}
	if IsNone("none") {
		t.Error("IsNone(\"none\") = true, want false")
	}
	if IsNone("default") {
		t.Error("IsNone(\"default\") = true, want false")
	}
}
