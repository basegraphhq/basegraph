import type React from "react"
import type { Metadata } from "next"
import { Lora, Geist_Mono } from "next/font/google"
import { Analytics } from "@vercel/analytics/next"
import "./globals.css"

const _lora = Lora({ subsets: ["latin"], variable: "--font-serif" })
const _geistMono = Geist_Mono({ subsets: ["latin"] })

export const metadata: Metadata = {
  title: "Basegraph - Human-Centered AI Code Editor",
  description:
    "Reimagining software development with intelligent collaboration between human creativity and machine precision.",
  generator: "v0.app",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en">
      <body className={`font-serif antialiased`}>
        {children}
        <Analytics />
      </body>
    </html>
  )
}
