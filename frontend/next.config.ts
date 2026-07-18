import path from "path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  outputFileTracingRoot: path.join(__dirname),
  async rewrites() {
    const backendUrl =
      process.env.NODE_ENV === "development"
        ? "http://localhost:8080"
        : "https://cielowave-api.joaquinvasquez.com";
    return [
      {
        source: "/api/:path*",
        destination: `${backendUrl}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;