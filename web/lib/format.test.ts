import { describe, expect, it } from "vitest";
import { formatDate, formatTime, formatVND, todayVN } from "./format";

describe("format", () => {
  it("formats VND (U-01 / W-15)", () => {
    expect(formatVND(35000)).toBe("35.000đ");
    expect(formatVND(0)).toBe("0đ");
    expect(formatVND(1250000)).toBe("1.250.000đ");
  });
  it("formats dates and times in Vietnam time", () => {
    // 16:30 UTC on 19/11 is 23:30 in Vietnam
    expect(formatDate("2026-11-19T16:30:00Z")).toBe("19/11/2026");
    expect(formatTime("2026-11-19T16:30:00Z")).toBe("23:30");
    expect(todayVN(new Date("2026-11-19T17:30:00Z"))).toBe("2026-11-20");
  });
});
