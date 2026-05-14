package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRangeTableDriven(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:           "valid explicit range",
			url:            "/comparison?from=2026-05-14T09:00:00Z&to=2026-05-14T10:00:00Z",
			wantStatusCode: 0,
			wantErr:        false,
		},
		{
			name:           "default range when missing",
			url:            "/comparison",
			wantStatusCode: 0,
			wantErr:        false,
		},
		{
			name:           "invalid from format",
			url:            "/comparison?from=bad&to=2026-05-14T10:00:00Z",
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
		},
		{
			name:           "invalid bounds from equals to",
			url:            "/comparison?from=2026-05-14T10:00:00Z&to=2026-05-14T10:00:00Z",
			wantStatusCode: http.StatusUnprocessableEntity,
			wantErr:        true,
		},
		{
			name:           "missing to",
			url:            "/comparison?from=2026-05-14T09:00:00Z",
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			from, to, apiErr := parseRange(req)
			if tc.wantErr {
				if apiErr == nil {
					t.Fatalf("expected error but got none")
				}
				if apiErr.StatusCode != tc.wantStatusCode {
					t.Fatalf("expected status %d, got %d", tc.wantStatusCode, apiErr.StatusCode)
				}
				return
			}
			if apiErr != nil {
				t.Fatalf("expected no error, got %+v", apiErr)
			}
			if !from.Before(to) {
				t.Fatalf("expected from < to, got from=%s to=%s", from, to)
			}
		})
	}
}
