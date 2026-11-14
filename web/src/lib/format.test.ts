import { describe, expect, it } from "vitest";

import { dayKey, dayParts, longDate, rupiah, timeLabel, upcomingDays } from "./format";

describe("venue formats", () => {
  it("reads the calendar day in WIB, not the device's zone", () => {
    // 23.30 WIB on 14 November is still the 14th in Jakarta, though 16.30 UTC.
    expect(dayKey(new Date("2026-11-14T16:30:00Z"))).toBe("2026-11-14");
    // 00.30 WIB on the 15th is the 14th in UTC.
    expect(dayKey(new Date("2026-11-14T17:30:00Z"))).toBe("2026-11-15");
  });

  it("lists the coming days across a month end", () => {
    expect(upcomingDays(new Date("2026-11-29T05:00:00Z"), 4)).toEqual([
      "2026-11-29",
      "2026-11-30",
      "2026-12-01",
      "2026-12-02",
    ]);
  });

  it("writes times, dates, and Rupiah the Indonesian way", () => {
    expect(timeLabel("2026-11-20T19:00:00+07:00")).toBe("19.00");
    expect(timeLabel("2026-11-20T12:00:00Z")).toBe("19.00");
    expect(longDate("2026-11-20T19:00:00+07:00")).toBe("Jumat, 20 November 2026");
    expect(rupiah(80000)).toBe("Rp80.000");
    expect(rupiah(1250000)).toBe("Rp1.250.000");
    expect(dayParts("2026-11-14")).toEqual({ weekday: "Sab", day: "14", month: "Nov" });
  });
});
