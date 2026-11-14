import type { Metadata, Viewport } from "next";
import { Archivo } from "next/font/google";
import type { ReactNode } from "react";

import { DemoBar } from "@/components/DemoBar";
import { SiteFooter } from "@/components/SiteFooter";

import "./globals.css";
import { Providers } from "./providers";

// One family; its width axis gives the wide scoreboard numerals (docs/design-brief.md).
const archivo = Archivo({
  subsets: ["latin"],
  axes: ["wdth"],
  variable: "--font-archivo",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Arena Kotabaru: pesan lapangan badminton dan futsal",
  description:
    "Lihat lapangan yang masih kosong, pesan, dan bayar DP dari HP. Demo portofolio dengan venue fiktif.",
  authors: [{ name: "Putu Jeevallucas Jnanamaitriya Surya Gautama" }],
};

export const viewport: Viewport = {
  themeColor: "#f4f6f1",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="id" className={archivo.variable}>
      <body>
        <DemoBar />
        <Providers>{children}</Providers>
        <SiteFooter />
      </body>
    </html>
  );
}
