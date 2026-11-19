import { apiError, callsTo, mockApi, renderWithClient } from "@/lib/admin/test-utils";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ownerMe, ql1Me } from "@/lib/admin/fixtures";
import { AdminShell } from "./AdminShell";
import { LoginForm } from "./LoginForm";

const replace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
  usePathname: () => "/admin",
}));

beforeEach(() => replace.mockReset());

async function fillAndSubmit(email: string, password: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Email"), email);
  await user.type(screen.getByLabelText("Mật khẩu"), password);
  await user.click(screen.getByRole("button", { name: "Đăng nhập" }));
}

describe("A-02 / screen 20: admin login", () => {
  it("shows a generic error with the remaining attempts", async () => {
    mockApi({
      "POST /auth/login": apiError(401, "INVALID_CREDENTIALS", "Sai thông tin đăng nhập", { remainingAttempts: 3 }),
    });
    renderWithClient(<LoginForm />);
    await fillAndSubmit("owner@moc.vn", "sai");
    expect(await screen.findByRole("alert")).toHaveTextContent("Email hoặc mật khẩu không đúng. Còn 3 lần thử trước khi tài khoản bị khoá 15 phút.");
    expect(callsTo("POST", "/auth/login")[0].body).toEqual({ email: "owner@moc.vn", password: "sai" });
    expect(replace).not.toHaveBeenCalled();
  });

  it("shows the lock time when the account is locked", async () => {
    mockApi({
      "POST /auth/login": apiError(423, "ACCOUNT_LOCKED", "Tài khoản tạm khoá", { lockedUntil: "2026-11-19T00:15:00Z" }),
    });
    renderWithClient(<LoginForm />);
    await fillAndSubmit("owner@moc.vn", "sai");
    expect(await screen.findByRole("alert")).toHaveTextContent("Tài khoản tạm khoá đến 07:15");
  });

  it("goes to the dashboard after a successful login", async () => {
    mockApi({ "POST /auth/login": { body: ownerMe } });
    renderWithClient(<LoginForm />);
    await fillAndSubmit("owner@moc.vn", "matkhau123");
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/admin"));
  });

  it("refuses POS-only accounts", async () => {
    mockApi({
      "POST /auth/login": { body: { ...ql1Me, user: { ...ql1Me.user, role: "cashier" } } },
      "POST /auth/logout": { status: 204 },
    });
    renderWithClient(<LoginForm />);
    await fillAndSubmit("tn1@moc.vn", "matkhau123");
    expect(await screen.findByRole("alert")).toHaveTextContent("chỉ dùng trên máy POS");
    expect(replace).not.toHaveBeenCalled();
  });
});

describe("admin guard", () => {
  it("redirects to the login page when /auth/me is 401", async () => {
    mockApi({ "GET /auth/me": apiError(401, "UNAUTHENTICATED", "Chưa đăng nhập") });
    renderWithClient(
      <AdminShell>
        <p>Nội dung</p>
      </AdminShell>,
    );
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/admin/dang-nhap"));
    expect(screen.queryByText("Nội dung")).not.toBeInTheDocument();
  });

  it("renders the sidebar and content for a signed-in owner", async () => {
    mockApi({ "GET /auth/me": { body: ownerMe } });
    renderWithClient(
      <AdminShell>
        <p>Nội dung</p>
      </AdminShell>,
    );
    expect(await screen.findByText("Nội dung")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Tổng quan" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByText("Tồn kho").closest("[aria-disabled]")).toHaveTextContent("GĐ2");
    expect(screen.getByRole("link", { name: "Cài đặt" })).toBeInTheDocument();
  });
});
