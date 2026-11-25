import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NODE_ENV === 'development' ? {
    async rewrites() { return [
      { source: '/api/:path*', destination: 'http://127.0.0.1:8080/api/:path*' },
      { source: '/upload-img/:path*', destination: 'http://127.0.0.1:8080/upload-img/:path*' },
    ]; },
  } : { output: 'export' as const }),
  distDir: process.env.NODE_ENV === 'development' ? '.next-dev' : '.next',
  onDemandEntries: { maxInactiveAge: 60 * 60 * 1000, pagesBufferLength: 64 },
  ...(process.env.NODE_ENV === 'development' ? { experimental: { staleTimes: { dynamic: 300 } } } : {}),
  trailingSlash: true,
};

export default nextConfig;
