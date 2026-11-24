import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "ORBIT — Distributed Key-Value Database",
  description:
    "A distributed database built to understand failure. Raft consensus, multi-shard consistent hashing, atomic LSM-Tree storage engine.",
  keywords: [
    "distributed systems",
    "raft consensus",
    "key-value database",
    "LSM tree",
    "sharding",
    "fault tolerance",
    "golang",
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark scroll-smooth">
      <head>
        <link rel="preconnect" href="https://api.fontshare.com" />
        <link rel="preconnect" href="https://cdn.fontshare.com" crossOrigin="anonymous" />
        <link
          rel="stylesheet"
          href="https://api.fontshare.com/v2/css?f[]=switzer@100,200,300,400,500,600,700,800,900&display=swap"
        />
      </head>
      <body className="bg-[#030712] text-slate-100 min-h-screen font-sans antialiased selection:bg-sky-500/30 selection:text-sky-200">
        {children}
      </body>
    </html>
  );
}
