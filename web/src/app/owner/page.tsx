import type { Metadata } from "next";
import { connection } from "next/server";

import { OwnerBoard } from "@/components/owner/OwnerBoard";

export const metadata: Metadata = {
  title: "Papan jadwal: Arena Kotabaru",
};

export default async function OwnerPage() {
  // The board opens on today in WIB; render per request so a build-time date never shows.
  await connection();
  return <OwnerBoard />;
}
