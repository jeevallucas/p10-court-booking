import type { NextConfig } from "next";

// The browser talks only to this app; /v1 and the fake payment page are proxied to the API so
// the owner's session cookie stays first-party (see docs/decisions.md).
const apiUrl = process.env.API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  // A self-contained server for the Docker image; Vercel builds the app its own way.
  output: "standalone",
  async rewrites() {
    return [
      { source: "/v1/:path*", destination: `${apiUrl}/v1/:path*` },
      { source: "/fakepay/:path*", destination: `${apiUrl}/fakepay/:path*` },
    ];
  },
};

export default nextConfig;
