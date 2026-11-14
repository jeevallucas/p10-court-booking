import type { Metadata } from "next";

import { BookingStatus } from "@/components/player/BookingStatus";

export const metadata: Metadata = {
  title: "Status booking: Arena Kotabaru",
};

export default async function BookingPage({ params }: { params: Promise<{ reference: string }> }) {
  const { reference } = await params;
  return <BookingStatus reference={reference.toUpperCase()} />;
}
