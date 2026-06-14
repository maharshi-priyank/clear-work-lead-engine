package leads

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	mw "github.com/amplexo/clearwork-leads-engine/internal/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/export", h.export)
	r.Post("/{id}/import", h.importOne)
	r.Post("/bulk-import", h.bulkImport)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	q := parseQuery(r)
	result, err := h.svc.FindAll(r.Context(), uid, q)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, result)
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	q := parseQuery(r)
	csv, err := h.svc.ExportCSV(r.Context(), uid, q)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="leads.csv"`)
	w.Write([]byte(csv))
}

func (h *Handler) importOne(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	id := chi.URLParam(r, "id")
	result, err := h.svc.ImportToCRM(r.Context(), uid, id)
	if err != nil {
		jsonError(w, err.Error(), 404)
		return
	}
	jsonOK(w, result)
}

func (h *Handler) bulkImport(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserID(r.Context())
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		jsonError(w, "ids array required", 400)
		return
	}
	result, err := h.svc.BulkImport(r.Context(), uid, body.IDs)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, result)
}

func parseQuery(r *http.Request) QueryParams {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	minScore, _ := strconv.Atoi(q.Get("minScore"))
	return QueryParams{
		CampaignID: q.Get("campaignId"),
		Source:     q.Get("source"),
		Search:     q.Get("search"),
		MinScore:   minScore,
		Imported:   q.Get("imported"),
		Page:       page,
		Limit:      limit,
	}
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
