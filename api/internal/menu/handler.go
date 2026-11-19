package menu

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

// Register adds the menu routes.
//
// The sold-out route uses the path parameter name ":id" for the branch so it
// shares the wildcard with the branches module's /branches/:id routes (gin
// rejects different wildcard names at the same position). The URL is the
// contract's /branches/{branchId}/sold-out/{productId}.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.GET("/menu", auth.RequireAuth(), h.getMenu)
	g.GET("/products", requireMenuViewer(), h.listProducts)
	g.POST("/products", auth.RequireAction(domain.ActEditMenu), h.createProduct)
	g.PUT("/products/:id", auth.RequireAction(domain.ActEditMenu), h.updateProduct)
	g.POST("/categories", auth.RequireAction(domain.ActEditMenu), h.createCategory)
	g.PUT("/categories/:id", auth.RequireAction(domain.ActEditMenu), h.updateCategory)
	g.PUT("/branches/:id/sold-out/:productId", auth.RequireAction(domain.ActSoldOut), h.setSoldOut)
}

type handler struct{ d *platform.Deps }

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// requireMenuViewer lets in users who edit the menu or toggle sold-out (the admin product list).
func requireMenuViewer() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.From(c)
		if p == nil {
			c.AbortWithStatusJSON(401, httpx.ErrorBody{Error: apperr.ErrUnauthenticated})
			return
		}
		if domain.Can(p.Role, domain.ActEditMenu) == domain.ScopeNone && domain.Can(p.Role, domain.ActSoldOut) == domain.ScopeNone {
			c.AbortWithStatusJSON(403, httpx.ErrorBody{Error: apperr.ErrForbidden})
			return
		}
		c.Next()
	}
}

func (h *handler) getMenu(c *gin.Context) {
	p := auth.From(c)
	var requested *int64
	if s := c.Query("branchId"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v <= 0 {
			httpx.Fail(c, apperr.BadRequest("VALIDATION_ERROR", "Mã chi nhánh không hợp lệ"))
			return
		}
		requested = &v
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := GetMenu(c.Request.Context(), store.New(h.d.Pool), branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) listProducts(c *gin.Context) {
	p := auth.From(c)
	ps, err := ListProducts(c.Request.Context(), store.New(h.d.Pool), p.BranchID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, ps)
}

func (h *handler) createProduct(c *gin.Context) {
	var in ProductInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := CreateProduct(c.Request.Context(), h.d, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateProduct(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in ProductInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := UpdateProduct(c.Request.Context(), h.d, auth.From(c).UserID, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createCategory(c *gin.Context) {
	var in CategoryInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := CreateCategory(c.Request.Context(), h.d, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateCategory(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in CategoryInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := UpdateCategory(c.Request.Context(), h.d, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) setSoldOut(c *gin.Context) {
	p := auth.From(c)
	branchID, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	productID, err := httpx.PathID(c, "productId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := auth.CheckBranch(p, branchID); err != nil {
		httpx.Fail(c, err)
		return
	}
	var in struct {
		SoldOut *bool `json:"soldOut"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if in.SoldOut == nil {
		httpx.Fail(c, apperr.BadRequest("VALIDATION_ERROR", "Thiếu trạng thái hết món"))
		return
	}
	if err := SetSoldOut(c.Request.Context(), h.d, p.UserID, branchID, productID, *in.SoldOut); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(204)
}
