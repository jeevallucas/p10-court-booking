import { describe, expect, it } from "vitest";

import { problemMessage, unreachableMessage } from "./problems";

describe("problemMessage", () => {
  it("explains a known API problem in Indonesian", () => {
    expect(problemMessage({ code: "slot_taken", detail: "slot is taken" })).toMatch(
      /baru saja dipesan orang lain/,
    );
  });

  it("treats a missing or non-problem body as the server being unreachable", () => {
    expect(problemMessage(undefined)).toBe(unreachableMessage);
    expect(problemMessage("<html>502 Bad Gateway</html>")).toBe(unreachableMessage);
    expect(problemMessage({ code: "teapot", detail: "?" })).toBe(unreachableMessage);
  });
});
