package campaign

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	mw "github.com/amplexo/clearwork-leads-engine/internal/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Post("/{id}/refetch", h.refetch)
	r.Delete("/{id}", h.delete)
	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	var dto CreateDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil || dto.Name == "" || len(dto.Providers) == 0 {
		jsonError(w, "name and providers are required", 400)
		return
	}
	c, err := h.svc.Create(r.Context(), uid, dto)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, c)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	campaigns, err := h.svc.FindAll(r.Context(), uid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, campaigns)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	id := chi.URLParam(r, "id")
	c, err := h.svc.FindOne(r.Context(), uid, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	jsonOK(w, c)
}

func (h *Handler) refetch(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	id := chi.URLParam(r, "id")
	c, err := h.svc.Refetch(r.Context(), uid, id)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	jsonOK(w, c)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Delete(r.Context(), uid, id); err != nil {
		jsonError(w, err.Error(), 404)
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
