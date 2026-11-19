import { act, cleanup, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useIdleLock } from "@/lib/pos/idle-lock";
import { deviceInfo, meLan } from "@/lib/pos/test-fixtures";
import { HttpError, mockApi, renderPos, routerMock } from "@/lib/pos/test-utils";
import { PinLoginScreen, pinErrorMessage } from "./PinLoginScreen";
import { ApiRequestError } from "@/lib/api/client";
import { posRedirect } from "./PosGate";

vi.mock("@/lib/api/client", async (orig) => ({ ...(await orig<object>()), api: (await import("@/lib/pos/test-utils")).mockApi.api }));
vi.mock("next/navigation", async () => {
  const { routerMock } = await import("@/lib/pos/test-utils");
  return { useRouter: () => routerMock };
});

async function pressPin(user: ReturnType<typeof userEvent.setup>, pin: string) {
  const pad = screen.getByRole("group", { name: "Bàn phím nhập PIN" });
  for (const d of pin) await user.click(within(pad).getByRole("button", { name: d }));
}

const dotsFilled = () => screen.getAllByTestId("pin-dot").filter((d) => d.dataset.filled === "true").length;

afterEach(cleanup);

describe("W-02 PIN login", () => {
  beforeEach(() => {
    mockApi.reset();
    vi.clearAllMocks();
    mockApi.on("GET", "/device", deviceInfo);
  });

  it("shows branch and device, enables Vào ca from 4 digits, logs in with the right PIN", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/auth/pin", ({ body }: { body?: unknown }) => {
      const b = body as { userId: number; pin: string };
      if (b.userId === 11 && b.pin === "1234") return meLan;
      throw new HttpError(401, "INVALID_PIN", "Sai PIN");
    });
    renderPos(<PinLoginScreen />);

    expect(await screen.findByTestId("device-branch")).toHaveTextContent("CN1 · iPad quầy 1");
    expect(screen.getByText("Tự khoá sau 15 phút không thao tác.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Lan/ }));
    expect(screen.getByRole("heading", { name: "Xin chào, Lan" })).toBeInTheDocument();

    const submit = screen.getByRole("button", { name: "Vào ca" });
    await pressPin(user, "123");
    expect(submit).toBeDisabled();
    await pressPin(user, "4");
    expect(submit).toBeEnabled();
    expect(dotsFilled()).toBe(4);
    await user.click(submit);

    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos"));
    expect(mockApi.callsTo("POST", "/auth/pin")[0].init.body).toEqual({ userId: 11, pin: "1234" });
  });

  it("wrong PIN shows an error and clears the input", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/auth/pin", () => {
      throw new HttpError(401, "INVALID_PIN", "Sai PIN");
    });
    renderPos(<PinLoginScreen />);
    await user.click(await screen.findByRole("button", { name: /Lan/ }));
    await pressPin(user, "0000");
    await user.click(screen.getByRole("button", { name: "Vào ca" }));

    expect(await screen.findByText("Mã PIN không đúng, vui lòng thử lại")).toBeInTheDocument();
    expect(dotsFilled()).toBe(0);
    expect(routerMock.replace).not.toHaveBeenCalled();
    // At most 6 digits
    await pressPin(user, "12345678");
    expect(dotsFilled()).toBe(6);
  });

  it("explains locked (423) and disabled (403) accounts", () => {
    const locked = new ApiRequestError(423, { code: "ACCOUNT_LOCKED", message: "x", details: { lockedUntil: "2026-11-19T00:15:00Z" } });
    expect(pinErrorMessage(locked)).toBe("Nhập sai PIN quá nhiều lần. Tài khoản tạm khoá đến 07:15.");
    expect(pinErrorMessage(new ApiRequestError(403, { code: "ACCOUNT_DISABLED", message: "x" }))).toBe("Tài khoản đã bị khoá. Liên hệ quản lý chi nhánh.");
  });

  it("sends a tablet without device to activation", async () => {
    mockApi.on("GET", "/device", () => {
      throw new HttpError(401, "DEVICE_REQUIRED");
    });
    renderPos(<PinLoginScreen />);
    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/kich-hoat"));
  });
});

describe("W-02 auto-lock after 15 minutes idle", () => {
  afterEach(() => vi.useRealTimers());

  it("locks after 15 idle minutes, and any touch restarts the timer", () => {
    vi.useFakeTimers();
    const onLock = vi.fn();
    renderHook(() => useIdleLock(15, onLock));
    act(() => vi.advanceTimersByTime(14 * 60_000));
    window.dispatchEvent(new Event("pointerdown"));
    act(() => vi.advanceTimersByTime(14 * 60_000));
    expect(onLock).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(60_000));
    expect(onLock).toHaveBeenCalledTimes(1);
  });
});

describe("/pos redirect", () => {
  const me = { user: { role: "cashier" }, device: {} };
  it("routes by device, session and shift", () => {
    expect(posRedirect({ device: null, me: null, shift: undefined })).toBe("/pos/kich-hoat");
    expect(posRedirect({ device: {}, me: null, shift: undefined })).toBe("/pos/dang-nhap");
    expect(posRedirect({ device: {}, me, shift: null })).toBe("/pos/mo-ca");
    expect(posRedirect({ device: {}, me, shift: {} })).toBe("/pos/ban");
    expect(posRedirect({ device: {}, me, shift: undefined })).toBeNull();
  });
});
