package api

import (
	"net/http"
)

// GetCatalog returns all categories, items, and tags.
func (h *Handler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.store.GetCatalog(r.Context())
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, catalog)
}

// GetItem returns a single item by ID.
func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request) {
	item := h.getItemOrError(w, r, "id")
	if item == nil {
		return
	}

	sendJSON(w, http.StatusOK, item)
}
