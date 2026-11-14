import { connection } from "next/server";

import { BookingPage } from "@/components/player/BookingPage";

export default async function Home() {
  // The date strip starts today in WIB; a page built once would keep showing the build day.
  await connection();
  return <BookingPage />;
}
