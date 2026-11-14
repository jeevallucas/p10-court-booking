"use client";

import { useMutation } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { api } from "@/api/client";
import { problemMessage } from "@/lib/problems";

import styles from "./LoginForm.module.css";

// The public demo account; the seed program creates it from DEMO_OWNER_PASSWORD (.env.example)
// and the nightly reseed restores it, so publishing it here costs nothing.
const demoAccount = { email: "demo@arenakotabaru.test", password: "arena-demo-2026" };

export function LoginForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const logIn = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.POST("/v1/owner/session", {
        body: { email: email.trim(), password },
      });
      if (response.status === 401) {
        throw new Error("Email atau password salah. Periksa lagi, lalu coba masuk.");
      }
      if (error) {
        throw new Error(problemMessage(error));
      }
    },
    onSuccess: () => router.push("/owner"),
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    logIn.mutate();
  }

  return (
    <main className={styles.page}>
      <form className={styles.form} onSubmit={submit}>
        <h1>Masuk sebagai pengelola</h1>
        <p className={styles.lead}>Papan jadwal Arena Kotabaru untuk meja depan.</p>

        <label className={styles.field}>
          Email
          <input
            type="email"
            name="email"
            autoComplete="username"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </label>
        <label className={styles.field}>
          Password
          <input
            type="password"
            name="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        {logIn.isError && (
          <p className={styles.failure} role="alert">
            {logIn.error.message}
          </p>
        )}

        <button type="submit" className={styles.primary} disabled={logIn.isPending}>
          {logIn.isPending ? "Masuk…" : "Masuk"}
        </button>

        <div className={styles.demo}>
          <p>
            Akun demo: <strong>{demoAccount.email}</strong>, password{" "}
            <strong>{demoAccount.password}</strong>. Data demo dikembalikan setiap malam.
          </p>
          <button
            type="button"
            onClick={() => {
              setEmail(demoAccount.email);
              setPassword(demoAccount.password);
            }}
          >
            Isi akun demo
          </button>
        </div>
      </form>
    </main>
  );
}
