/** @type {import('next').NextConfig} */
const nextConfig = {
  typescript: {
    ignoreBuildErrors: true,
  },
  images: {
    unoptimized: true,
  },
  rewrites: async () => {
    return {
      beforeFiles: [
        {
          source: '/callback',
          destination: 'http://129.159.179.42:8080/api/auth/tidal/callback',
        },
      ],
    }
  },
}

export default nextConfig
