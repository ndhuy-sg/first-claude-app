import { apiError, callsTo, mockApi, renderAdmin, setRoute } from "@/lib/admin/test-utils";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { allBranches, caPheSua, cn1, menu, ownerMe, ql1Me } from "@/lib/admin/fixtures";
import { ProductEdit } from "./ProductEdit";
import { ProductsPage } from "./ProductsPage";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  usePathname: () => "/admin/mon",
  useParams: () => ({ id: "10" }),
}));

describe("W-11 (UI half): owner creates a product", () => {
  it("posts the product and it appears in the list", async () => {
    let products: Schemas["Product"][] = [caPheSua];
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /menu": { body: menu },
      "GET /products": () => ({ body: products }),
      "POST /products": (req) => {
        const body = req.body as Schemas["ProductInput"];
        const created: Schemas["Product"] = {
          ...caPheSua,
          id: 99,
          name: body.name,
          categoryId: body.categoryId,
          basePrice: body.basePrice,
          sizes: [],
          modifiers: [],
          isFeatured: false,
        };
        products = [...products, created];
        return { status: 201, body: created };
      },
    });
    renderAdmin(<ProductsPage />, ownerMe);
    const user = userEvent.setup();
    await screen.findByText("Cà phê sữa");

    await user.click(screen.getByRole("button", { name: "+ Thêm món" }));
    const form = screen.getByRole("form", { name: "Thêm món mới" });
    await user.type(within(form).getByLabelText("Tên món"), "Bạc xỉu");
    await waitFor(() => expect(within(form).getByLabelText("Nhóm món")).toHaveValue("1"));
    await user.selectOptions(within(form).getByLabelText("Nhóm món"), "2");
    await user.type(within(form).getByLabelText("Giá gốc (đồng, đã gồm thuế)"), "32000");
    expect(within(form).getByText("Giá bán: 32.000đ")).toBeInTheDocument();
    await user.click(within(form).getByRole("button", { name: "Lưu món" }));

    expect(await screen.findByRole("status")).toHaveTextContent("Đã thêm món “Bạc xỉu”");
    expect(callsTo("POST", "/products")[0].body).toEqual({
      categoryId: 2,
      name: "Bạc xỉu",
      basePrice: 32000,
      description: "",
      isActive: true,
      sizes: [],
      modifiers: [],
    });
    const table = screen.getByRole("table", { name: "Danh sách món" });
    await waitFor(() => expect(table).toHaveTextContent("Bạc xỉu"));
    expect(table).toHaveTextContent("32.000đ");
    expect(table).toHaveTextContent("Trà");
  });

  it("rejects an invalid price and shows the API conflict message", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /menu": { body: menu },
      "GET /products": { body: [caPheSua] },
      "POST /products": apiError(409, "PRODUCT_NAME_TAKEN", "Tên món đã có trong nhóm"),
    });
    renderAdmin(<ProductsPage />, ownerMe);
    const user = userEvent.setup();
    await screen.findByText("Cà phê sữa");
    await user.click(screen.getByRole("button", { name: "+ Thêm món" }));
    const form = screen.getByRole("form", { name: "Thêm món mới" });
    await user.type(within(form).getByLabelText("Tên món"), "Cà phê sữa");
    await user.type(within(form).getByLabelText("Giá gốc (đồng, đã gồm thuế)"), "abc");
    await user.click(within(form).getByRole("button", { name: "Lưu món" }));
    expect(within(form).getByRole("alert")).toHaveTextContent("Giá phải là số nguyên ≥ 0");
    expect(callsTo("POST", "/products")).toHaveLength(0);

    await user.clear(within(form).getByLabelText("Giá gốc (đồng, đã gồm thuế)"));
    await user.type(within(form).getByLabelText("Giá gốc (đồng, đã gồm thuế)"), "29.000");
    await user.click(within(form).getByRole("button", { name: "Lưu món" }));
    expect(await within(form).findByText("Tên món đã có trong nhóm")).toBeInTheDocument();
  });

  it("branch manager cannot add products but toggles sold-out for CN1 only", async () => {
    mockApi({
      "GET /branches": { body: [cn1] },
      "GET /menu": { body: menu },
      "GET /products": { body: [caPheSua] },
      "PUT /branches/1/sold-out/10": { status: 204 },
    });
    renderAdmin(<ProductsPage />, ql1Me);
    await screen.findByText("Cà phê sữa");
    expect(screen.queryByRole("button", { name: "+ Thêm món" })).not.toBeInTheDocument();
    expect(callsTo("GET", "/menu")[0].query.branchId).toBe("1");
    const toggle = screen.getByRole("button", { name: "Còn món" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(toggle);
    await waitFor(() => expect(callsTo("PUT", "/branches/1/sold-out/10")[0]?.body).toEqual({ soldOut: true }));
  });
});

describe("screen 27: product edit", () => {
  it("edits price, sizes and modifiers; recipe is a disabled phase-2 section", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /menu": { body: menu },
      "GET /products": { body: [caPheSua] },
    });
    setRoute("PUT /products/10", (req) => ({ body: { ...caPheSua, ...(req.body as object) } }));
    renderAdmin(<ProductEdit />, ownerMe);
    const user = userEvent.setup();
    const price = await screen.findByLabelText("Giá gốc (đồng, đã gồm thuế)");
    await user.clear(price);
    await user.type(price, "31000");
    await user.click(screen.getByRole("button", { name: "+ Thêm size" }));
    await user.type(screen.getByLabelText("Mã size 3"), "XL");
    await user.type(screen.getByLabelText("Giá cộng thêm size 3"), "0");
    await user.click(screen.getByRole("button", { name: "Xoá tuỳ chọn 2" }));
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    await waitFor(() => expect(callsTo("PUT", "/products/10")).toHaveLength(1));
    const body = callsTo("PUT", "/products/10")[0].body as Schemas["ProductInput"];
    expect(body.basePrice).toBe(31000);
    expect(body.sizes).toEqual([
      { code: "M", name: "M", priceDelta: 0 },
      { code: "L", name: "L", priceDelta: 6000 },
      { code: "XL", name: "XL", priceDelta: 0 },
    ]);
    expect(body.modifiers).toEqual([{ id: 1, name: "Thêm shot", price: 8000 }]);

    const recipe = screen.getByRole("region", { name: "Công thức món" });
    expect(recipe).toHaveAttribute("aria-disabled", "true");
    expect(recipe).toHaveTextContent("Giai đoạn 2");
    expect(screen.getAllByRole("button", { pressed: false }).map((b) => b.textContent)).toEqual([
      "CN1 – Mộc Nguyễn Huệ: Còn món",
      "CN2 – Mộc Lê Lợi: Còn món",
    ]);
  });
});
