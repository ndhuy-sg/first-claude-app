// Package menu serves the chain-wide menu (one price list for every branch),
// its admin editing (owner only) and the per-branch sold-out switch.
package menu

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

// ---------- JSON shapes (openapi.yaml: Menu, MenuProduct, Size, Modifier, Category, Product) ----------

type Size struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	PriceDelta int64  `json:"priceDelta"`
}

type Modifier struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int32  `json:"sortOrder"`
}

type MenuProduct struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	BasePrice   int64      `json:"basePrice"`
	IsFeatured  bool       `json:"isFeatured"`
	SoldOut     bool       `json:"soldOut"`
	Sizes       []Size     `json:"sizes"`
	Modifiers   []Modifier `json:"modifiers"`
}

type MenuCategory struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Products []MenuProduct `json:"products"`
}

type Menu struct {
	Categories []MenuCategory `json:"categories"`
}

type Product struct {
	ID               int64      `json:"id"`
	CategoryID       int64      `json:"categoryId"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	BasePrice        int64      `json:"basePrice"`
	IsActive         bool       `json:"isActive"`
	IsFeatured       bool       `json:"isFeatured"`
	SortOrder        int32      `json:"sortOrder"`
	Sizes            []Size     `json:"sizes"`
	Modifiers        []Modifier `json:"modifiers"`
	SoldOutBranchIDs []int64    `json:"soldOutBranchIds"`
}

// ---------- inputs ----------

type CategoryInput struct {
	Name      string `json:"name"`
	SortOrder *int32 `json:"sortOrder"`
}

type ModifierInput struct {
	ID    *int64 `json:"id"`
	Name  string `json:"name"`
	Price *int64 `json:"price"`
}

type ProductInput struct {
	CategoryID  *int64          `json:"categoryId"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	BasePrice   *int64          `json:"basePrice"`
	IsActive    *bool           `json:"isActive"`
	IsFeatured  *bool           `json:"isFeatured"`
	SortOrder   *int32          `json:"sortOrder"`
	Sizes       []Size          `json:"sizes"`     // nil (omitted) on update = keep current sizes
	Modifiers   []ModifierInput `json:"modifiers"` // nil (omitted) on update = keep current modifiers
}

func invalid(msg string) *apperr.Error { return apperr.BadRequest("VALIDATION_ERROR", msg) }

var (
	errProductNameTaken  = apperr.Conflict("PRODUCT_NAME_TAKEN", "Tên món đã có trong nhóm này")
	errCategoryNameTaken = apperr.Conflict("CATEGORY_NAME_TAKEN", "Tên nhóm món đã tồn tại")
)

func (in *CategoryInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("Vui lòng nhập tên nhóm món")
	}
	return nil
}

func (in *ProductInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.CategoryID == nil {
		return invalid("Vui lòng chọn nhóm món")
	}
	if in.Name == "" {
		return invalid("Vui lòng nhập tên món")
	}
	if in.BasePrice == nil {
		return invalid("Vui lòng nhập giá món")
	}
	if *in.BasePrice < 0 {
		return invalid("Giá món phải là số nguyên không âm")
	}
	codes := map[string]bool{}
	for i := range in.Sizes {
		s := &in.Sizes[i]
		s.Code, s.Name = strings.TrimSpace(s.Code), strings.TrimSpace(s.Name)
		if s.Code == "" || s.Name == "" {
			return invalid("Size phải có mã và tên")
		}
		if s.PriceDelta < 0 {
			return invalid("Giá size phải là số nguyên không âm")
		}
		if codes[s.Code] {
			return invalid("Mã size bị trùng: " + s.Code)
		}
		codes[s.Code] = true
	}
	names := map[string]bool{}
	ids := map[int64]bool{}
	for i := range in.Modifiers {
		m := &in.Modifiers[i]
		m.Name = strings.TrimSpace(m.Name)
		if m.Name == "" {
			return invalid("Tuỳ chọn phải có tên")
		}
		if m.Price == nil || *m.Price < 0 {
			return invalid("Giá tuỳ chọn phải là số nguyên không âm")
		}
		if names[m.Name] {
			return invalid("Tên tuỳ chọn bị trùng: " + m.Name)
		}
		names[m.Name] = true
		if m.ID != nil {
			if ids[*m.ID] {
				return invalid("Tuỳ chọn bị lặp lại")
			}
			ids[*m.ID] = true
		}
	}
	return nil
}

// mapUnique turns unique violations into contract errors.
func mapUnique(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		switch pg.ConstraintName {
		case "products_category_id_name_key":
			return errProductNameTaken
		case "categories_name_key":
			return errCategoryNameTaken
		case "modifiers_product_id_name_key":
			return invalid("Tên tuỳ chọn bị trùng")
		}
	}
	return err
}

// ---------- reads ----------

type catalog struct {
	sizes   map[int64][]Size
	mods    map[int64][]Modifier
	soldOut map[int64][]int64 // product id → branch ids
}

func loadCatalog(ctx context.Context, q *store.Queries, productID, soldOutBranch *int64) (*catalog, error) {
	c := &catalog{sizes: map[int64][]Size{}, mods: map[int64][]Modifier{}, soldOut: map[int64][]int64{}}
	sizes, err := q.MenuListSizes(ctx, productID)
	if err != nil {
		return nil, err
	}
	for _, s := range sizes {
		c.sizes[s.ProductID] = append(c.sizes[s.ProductID], Size{Code: s.Code, Name: s.Name, PriceDelta: s.PriceDelta})
	}
	mods, err := q.MenuListModifiers(ctx, store.MenuListModifiersParams{ProductID: productID, ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		c.mods[m.ProductID] = append(c.mods[m.ProductID], Modifier{ID: m.ID, Name: m.Name, Price: m.Price})
	}
	so, err := q.MenuListSoldOut(ctx, soldOutBranch)
	if err != nil {
		return nil, err
	}
	for _, r := range so {
		c.soldOut[r.ProductID] = append(c.soldOut[r.ProductID], r.BranchID)
	}
	return c, nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (c *catalog) product(p store.Product) Product {
	return Product{
		ID: p.ID, CategoryID: p.CategoryID, Name: p.Name, Description: p.Description, BasePrice: p.BasePrice,
		IsActive: p.IsActive, IsFeatured: p.IsFeatured, SortOrder: p.SortOrder,
		Sizes: orEmpty(c.sizes[p.ID]), Modifiers: orEmpty(c.mods[p.ID]), SoldOutBranchIDs: orEmpty(c.soldOut[p.ID]),
	}
}

// GetMenu returns active products grouped by category in sort order. soldOut is
// computed for branchID; with no branch every product is available.
// Categories without active products are left out.
func GetMenu(ctx context.Context, q *store.Queries, branchID *int64) (Menu, error) {
	cats, err := q.MenuListCategories(ctx)
	if err != nil {
		return Menu{}, err
	}
	prods, err := q.MenuListProducts(ctx, true)
	if err != nil {
		return Menu{}, err
	}
	branch := int64(0) // no branch: no sold-out rows match, everything is available
	if branchID != nil {
		branch = *branchID
	}
	c, err := loadCatalog(ctx, q, nil, &branch)
	if err != nil {
		return Menu{}, err
	}
	byCat := map[int64][]MenuProduct{}
	for _, p := range prods {
		byCat[p.CategoryID] = append(byCat[p.CategoryID], MenuProduct{
			ID: p.ID, Name: p.Name, Description: p.Description, BasePrice: p.BasePrice, IsFeatured: p.IsFeatured,
			SoldOut: len(c.soldOut[p.ID]) > 0, Sizes: orEmpty(c.sizes[p.ID]), Modifiers: orEmpty(c.mods[p.ID]),
		})
	}
	out := Menu{Categories: []MenuCategory{}}
	for _, cat := range cats {
		if ps := byCat[cat.ID]; len(ps) > 0 {
			out.Categories = append(out.Categories, MenuCategory{ID: cat.ID, Name: cat.Name, Products: ps})
		}
	}
	return out, nil
}

// ListProducts returns every product (hidden ones too). soldOutBranch limits
// soldOutBranchIds to one branch (branch-bound users); nil shows all branches.
func ListProducts(ctx context.Context, q *store.Queries, soldOutBranch *int64) ([]Product, error) {
	prods, err := q.MenuListProducts(ctx, false)
	if err != nil {
		return nil, err
	}
	c, err := loadCatalog(ctx, q, nil, soldOutBranch)
	if err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(prods))
	for _, p := range prods {
		out = append(out, c.product(p))
	}
	return out, nil
}

func getProduct(ctx context.Context, q *store.Queries, id int64) (Product, error) {
	p, err := q.MenuGetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	c, err := loadCatalog(ctx, q, &id, nil)
	if err != nil {
		return Product{}, err
	}
	return c.product(p), nil
}

// ---------- category writes ----------

func CreateCategory(ctx context.Context, d *platform.Deps, in CategoryInput) (Category, error) {
	if err := in.validate(); err != nil {
		return Category{}, err
	}
	var sort int32
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	c, err := store.New(d.Pool).MenuCreateCategory(ctx, store.MenuCreateCategoryParams{Name: in.Name, SortOrder: sort})
	if err != nil {
		return Category{}, mapUnique(err)
	}
	return Category{ID: c.ID, Name: c.Name, SortOrder: c.SortOrder}, nil
}

func UpdateCategory(ctx context.Context, d *platform.Deps, id int64, in CategoryInput) (Category, error) {
	if err := in.validate(); err != nil {
		return Category{}, err
	}
	var out Category
	err := store.InTx(ctx, d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		cats, err := q.MenuListCategories(ctx)
		if err != nil {
			return err
		}
		sort, found := int32(0), false
		for _, c := range cats {
			if c.ID == id {
				sort, found = c.SortOrder, true
			}
		}
		if !found {
			return apperr.ErrNotFound
		}
		if in.SortOrder != nil {
			sort = *in.SortOrder
		}
		c, err := q.MenuUpdateCategory(ctx, store.MenuUpdateCategoryParams{ID: id, Name: in.Name, SortOrder: sort})
		if err != nil {
			return mapUnique(err)
		}
		out = Category{ID: c.ID, Name: c.Name, SortOrder: c.SortOrder}
		return nil
	})
	return out, err
}

// ---------- product writes ----------

// priceSnapshot is what the product.price audit records before and after.
type priceSnapshot struct {
	BasePrice int64      `json:"basePrice"`
	Sizes     []Size     `json:"sizes"`
	Modifiers []Modifier `json:"modifiers"`
}

func (s priceSnapshot) key() (int64, map[string]int64, map[int64]int64) {
	sz := map[string]int64{}
	for _, x := range s.Sizes {
		sz[x.Code] = x.PriceDelta
	}
	md := map[int64]int64{}
	for _, x := range s.Modifiers {
		md[x.ID] = x.Price
	}
	return s.BasePrice, sz, md
}

func pricesDiffer(a, b priceSnapshot) bool {
	a1, a2, a3 := a.key()
	b1, b2, b3 := b.key()
	return a1 != b1 || !reflect.DeepEqual(a2, b2) || !reflect.DeepEqual(a3, b3)
}

func CreateProduct(ctx context.Context, d *platform.Deps, in ProductInput) (Product, error) {
	if err := in.validate(); err != nil {
		return Product{}, err
	}
	var id int64
	err := store.InTx(ctx, d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		ok, err := q.MenuCategoryExists(ctx, *in.CategoryID)
		if err != nil {
			return err
		}
		if !ok {
			return invalid("Nhóm món không tồn tại")
		}
		p := store.MenuInsertProductParams{
			CategoryID: *in.CategoryID, Name: in.Name, BasePrice: *in.BasePrice, IsActive: true, UpdatedAt: d.Clock.Now(),
		}
		if in.Description != nil {
			p.Description = *in.Description
		}
		if in.IsActive != nil {
			p.IsActive = *in.IsActive
		}
		if in.IsFeatured != nil {
			p.IsFeatured = *in.IsFeatured
		}
		if in.SortOrder != nil {
			p.SortOrder = *in.SortOrder
		}
		row, err := q.MenuInsertProduct(ctx, p)
		if err != nil {
			return mapUnique(err)
		}
		id = row.ID
		if err := replaceSizes(ctx, q, id, in.Sizes); err != nil {
			return err
		}
		return replaceModifiers(ctx, q, id, in.Modifiers)
	})
	if err != nil {
		return Product{}, err
	}
	return getProduct(ctx, store.New(d.Pool), id)
}

func UpdateProduct(ctx context.Context, d *platform.Deps, actorID, id int64, in ProductInput) (Product, error) {
	if err := in.validate(); err != nil {
		return Product{}, err
	}
	err := store.InTx(ctx, d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		cur, err := q.MenuGetProductForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound
		}
		if err != nil {
			return err
		}
		ok, err := q.MenuCategoryExists(ctx, *in.CategoryID)
		if err != nil {
			return err
		}
		if !ok {
			return invalid("Nhóm món không tồn tại")
		}
		before, err := getProduct(ctx, q, id)
		if err != nil {
			return err
		}
		p := store.MenuUpdateProductParams{
			ID: id, CategoryID: *in.CategoryID, Name: in.Name, Description: cur.Description, BasePrice: *in.BasePrice,
			IsActive: cur.IsActive, IsFeatured: cur.IsFeatured, SortOrder: cur.SortOrder, UpdatedAt: d.Clock.Now(),
		}
		if in.Description != nil {
			p.Description = *in.Description
		}
		if in.IsActive != nil {
			p.IsActive = *in.IsActive
		}
		if in.IsFeatured != nil {
			p.IsFeatured = *in.IsFeatured
		}
		if in.SortOrder != nil {
			p.SortOrder = *in.SortOrder
		}
		if _, err := q.MenuUpdateProduct(ctx, p); err != nil {
			return mapUnique(err)
		}
		if in.Sizes != nil {
			if err := replaceSizes(ctx, q, id, in.Sizes); err != nil {
				return err
			}
		}
		if in.Modifiers != nil {
			if err := replaceModifiers(ctx, q, id, in.Modifiers); err != nil {
				return err
			}
		}
		after, err := getProduct(ctx, q, id)
		if err != nil {
			return err
		}
		b := priceSnapshot{BasePrice: before.BasePrice, Sizes: before.Sizes, Modifiers: before.Modifiers}
		a := priceSnapshot{BasePrice: after.BasePrice, Sizes: after.Sizes, Modifiers: after.Modifiers}
		if pricesDiffer(b, a) {
			return platform.Audit(ctx, q, platform.AuditEntry{
				ActorID: &actorID, Action: "product.price", EntityType: "product", EntityID: itoa(id),
				Before: b, After: a, Note: after.Name,
			})
		}
		return nil
	})
	if err != nil {
		return Product{}, err
	}
	return getProduct(ctx, store.New(d.Pool), id)
}

func replaceSizes(ctx context.Context, q *store.Queries, productID int64, sizes []Size) error {
	if err := q.MenuDeleteSizes(ctx, productID); err != nil {
		return err
	}
	for i, s := range sizes {
		if err := q.MenuInsertSize(ctx, store.MenuInsertSizeParams{
			ProductID: productID, Code: s.Code, Name: s.Name, PriceDelta: s.PriceDelta, SortOrder: int32(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

// replaceModifiers makes the product's active modifiers exactly mods, in order.
// Existing rows are kept (by id, or by name when no id is given) so order
// snapshots that reference them stay valid; modifiers left out are deactivated,
// never deleted.
func replaceModifiers(ctx context.Context, q *store.Queries, productID int64, mods []ModifierInput) error {
	existing, err := q.MenuListModifiers(ctx, store.MenuListModifiersParams{ProductID: &productID})
	if err != nil {
		return err
	}
	byID := map[int64]store.Modifier{}
	for _, m := range existing {
		byID[m.ID] = m
	}
	claimed := map[int64]bool{}
	for _, m := range mods {
		if m.ID != nil {
			if _, ok := byID[*m.ID]; !ok {
				return invalid("Tuỳ chọn không thuộc món này")
			}
			claimed[*m.ID] = true
		}
	}
	byName := map[string]int64{}
	for _, m := range existing {
		if !claimed[m.ID] {
			byName[m.Name] = m.ID
		}
	}
	keep := map[int64]bool{}
	ids := make([]*int64, len(mods))
	for i, m := range mods {
		if m.ID != nil {
			ids[i] = m.ID
		} else if id, ok := byName[m.Name]; ok {
			ids[i] = &id
			delete(byName, m.Name)
		}
		if ids[i] != nil {
			keep[*ids[i]] = true
		}
	}
	// deactivate first so a removed name can be taken by another row
	for _, m := range existing {
		if m.IsActive && !keep[m.ID] {
			if err := q.MenuDeactivateModifier(ctx, m.ID); err != nil {
				return err
			}
		}
	}
	for i, m := range mods {
		if ids[i] != nil {
			if err := q.MenuUpdateModifier(ctx, store.MenuUpdateModifierParams{
				ID: *ids[i], Name: m.Name, Price: *m.Price, SortOrder: int32(i),
			}); err != nil {
				return mapUnique(err)
			}
			continue
		}
		if _, err := q.MenuInsertModifier(ctx, store.MenuInsertModifierParams{
			ProductID: productID, Name: m.Name, Price: *m.Price, SortOrder: int32(i),
		}); err != nil {
			return mapUnique(err)
		}
	}
	return nil
}

// ---------- sold-out ----------

// SoldOutEvent is the data of the product.sold_out event.
type SoldOutEvent struct {
	ProductID int64 `json:"productId"`
	SoldOut   bool  `json:"soldOut"`
}

// SetSoldOut marks or clears sold-out for one product in one branch. Branch
// access must be checked by the caller. A real change is audited and published.
func SetSoldOut(ctx context.Context, d *platform.Deps, actorID, branchID, productID int64, soldOut bool) error {
	changed := false
	err := store.InTx(ctx, d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		ok, err := q.MenuBranchExists(ctx, branchID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.ErrNotFound
		}
		if ok, err = q.MenuProductExists(ctx, productID); err != nil {
			return err
		}
		if !ok {
			return apperr.ErrNotFound
		}
		var n int64
		if soldOut {
			n, err = q.MenuMarkSoldOut(ctx, store.MenuMarkSoldOutParams{BranchID: branchID, ProductID: productID, MarkedBy: actorID, MarkedAt: d.Clock.Now()})
		} else {
			n, err = q.MenuUnmarkSoldOut(ctx, store.MenuUnmarkSoldOutParams{BranchID: branchID, ProductID: productID})
		}
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &actorID, BranchID: &branchID, Action: "product.sold_out", EntityType: "product", EntityID: itoa(productID),
			Before: map[string]bool{"soldOut": !soldOut}, After: map[string]bool{"soldOut": soldOut},
		})
	})
	if err != nil {
		return err
	}
	if changed {
		d.Events.Publish(platform.Event{Type: "product.sold_out", BranchID: branchID, Data: SoldOutEvent{ProductID: productID, SoldOut: soldOut}})
	}
	return nil
}
