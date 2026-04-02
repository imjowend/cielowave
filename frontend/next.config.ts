import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: "http://129.159.179.42:8080/api/:path*",
      },
    ];
  },
};

export default nextConfig;