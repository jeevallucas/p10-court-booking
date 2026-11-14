import createClient from "openapi-fetch";

import { unreachableMessage } from "@/lib/problems";

import type { components, paths } from "./schema";

// Same origin: next.config.ts proxies /v1 to the API. The base is absolute because
// Request outside a browser (Node, in tests) rejects relative URLs.
export const api = createClient<paths>({
  baseUrl: typeof window === "undefined" ? "" : window.location.origin,
});

// A dropped connection rejects fetch itself; give the player the same message as an outage.
api.use({ onError: () => new Error(unreachableMessage) });

export type Availability = components["schemas"]["Availability"];
export type Booking = components["schemas"]["Booking"];
export type Court = components["schemas"]["Court"];
export type Problem = components["schemas"]["Problem"];
export type Slot = components["schemas"]["Slot"];
export type Sport = components["schemas"]["Sport"];
