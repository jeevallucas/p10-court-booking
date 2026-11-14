import type { Metadata } from "next";

import { LoginForm } from "@/components/owner/LoginForm";

export const metadata: Metadata = {
  title: "Masuk pengelola: Arena Kotabaru",
};

export default function OwnerLoginPage() {
  return <LoginForm />;
}
