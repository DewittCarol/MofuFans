import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NODE_ENV === 'development' ? {
    async rewrites() { return [
      { source: '/api/:path*', destination: 'http://127.0.0.1:8080/api/:path*', basePath: false as const },
      { source: '/upload-img/:path*', destination: 'http://127.0.0.1:8080/upload-img/:path*', basePath: false as const },
    ]; },
  } : { output: 'export' as const }),
  // Keep live editing separate from the validation build; a build must not evict dev pages.
  distDir: process.env.NODE_ENV === 'development' ? '.next-dev' : '.next',
  onDemandEntries: { maxInactiveAge: 60 * 60 * 1000, pagesBufferLength: 64 },
  // Reuse route payloads when switching back and forth in the remote preview.
  // Page data still comes from authenticated, uncached client API requests.
  ...(process.env.NODE_ENV === 'development' ? { experimental: { staleTimes: { dynamic: 300 } } } : {}),
  basePath: '/admin',
  trailingSlash: true,
};

export default nextConfig;
