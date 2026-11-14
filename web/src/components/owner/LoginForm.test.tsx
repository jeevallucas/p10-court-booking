import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { answerWith, json } from "@/test/fakeApi";
import { renderWithQueries } from "@/test/render";

import { LoginForm } from "./LoginForm";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: () => {}, replace: () => {} }) }));

describe("LoginForm", () => {
  it("says the email or password is wrong, not that a session ended", async () => {
    answerWith(() => json(401, { code: "unauthorized", detail: "invalid credentials" }));
    renderWithQueries(<LoginForm />);

    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "owner@example.test" } });
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: "Masuk" }));

    expect((await screen.findByRole("alert")).textContent).toMatch(/Email atau password salah/);
  });

  it("tells the owner to wait after too many attempts", async () => {
    answerWith(() => json(429, { code: "rate_limited", detail: "slow down" }));
    renderWithQueries(<LoginForm />);

    fireEvent.click(screen.getByRole("button", { name: "Isi akun demo" }));
    fireEvent.click(screen.getByRole("button", { name: "Masuk" }));

    expect((await screen.findByRole("alert")).textContent).toMatch(/Tunggu satu menit/);
  });
});
