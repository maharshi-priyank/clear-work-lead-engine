package vault

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	mw "github.com/amplexo/clearwork-leads-engine/internal/middleware"
	"github.com/amplexo/clearwork-leads-engine/internal/providers"
)

type Handler struct {
	svc      *Service
	registry *providers.Registry
}

func NewHandler(svc *Service, registry *providers.Registry) *Handler {
	return &Handler{svc: svc, registry: registry}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/{provider}", h.save)
	r.Post("/{provider}/test", h.testKey)
	r.Delete("/{provider}", h.remove)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	saved, err := h.svc.List(r.Context(), uid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	// Merge with full catalog so UI sees all providers
	catalog := h.registry.Catalog()
	savedMap := make(map[string]ProviderKeyRow, len(saved))
	for _, s := range saved {
		savedMap[s.Provider] = s
	}
	type row struct {
		providers.CatalogEntry
		Connected   bool    `json:"connected"`
		Status      *string `json:"status"`
		LastTestedAt *string `json:"lastTestedAt"`
	}
	out := make([]row, 0, len(catalog))
	for _, c := range catalog {
		r2 := row{CatalogEntry: c}
		if s, ok := savedMap[c.Provider]; ok {
			r2.Connected = true
			r2.Status = &s.Status
			r2.LastTestedAt = s.LastTestedAt
		}
		out = append(out, r2)
	}
	jsonOK(w, out)
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	provider := chi.URLParam(r, "provider")

	adapter := h.registry.Get(provider)
	if adapter == nil {
		jsonError(w, "unknown provider", 400)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
		jsonError(w, "key is required", 400)
		return
	}

	// Test the key but don't block saving — let user save and test separately
	status := "active"
	ok, _ := adapter.TestKey(r.Context(), body.Key)
	if !ok {
		status = "unverified"
	}

	if err := h.svc.SaveWithStatus(r.Context(), uid, provider, body.Key, status); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"provider": provider, "status": status})
}

func (h *Handler) testKey(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	provider := chi.URLParam(r, "provider")

	adapter := h.registry.Get(provider)
	if adapter == nil {
		jsonError(w, "unknown provider", 400)
		return
	}
	rawKey, err := h.svc.GetDecryptedKey(r.Context(), uid, provider)
	if err != nil || rawKey == "" {
		jsonError(w, "no key saved for provider", 404)
		return
	}
	ok, testErr := adapter.TestKey(r.Context(), rawKey)
	status := "active"
	if !ok {
		status = "invalid"
	}
	_ = h.svc.UpdateStatus(r.Context(), uid, provider, status)

	resp := map[string]any{"provider": provider, "valid": ok, "status": status}
	if testErr != nil {
		resp["error"] = testErr.Error()
	}
	jsonOK(w, resp)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	provider := chi.URLParam(r, "provider")
	if err := h.svc.Remove(r.Context(), uid, provider); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
