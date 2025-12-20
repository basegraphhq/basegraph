import type { Metadata, Viewport } from "next";
import { Geist_Mono } from "next/font/google";
import type React from "react";
import "@fontsource/libertinus-serif";
import { Analytics } from "@vercel/analytics/next";
import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/toaster";
import "./globals.css";

const _geistMono = Geist_Mono({ subsets: ["latin"] });

export const metadata: Metadata = {
	metadataBase: new URL("https://basegraph.app"),
	title: {
		default: "Relay by Basegraph - Turn Tickets into Production-Ready Specs",
		template: "%s | Relay by Basegraph",
	},
	description:
		"Turn tickets into production-ready specs. Relay analyzes your tickets, gathers context from your team, and maps your codebase to generate specs that actually ship. Powered by deterministic code analysis.",
	keywords: [
		"AI code analysis",
		"Linear integration",
		"GitLab integration",
		"GitHub integration",
		"production specs",
		"code context",
		"developer tools",
		"AI agents",
		"codegraph",
		"static analysis",
		"software development",
		"ticket automation",
		"spec generation",
		"codebase mapping",
	],
	authors: [{ name: "Basegraph", url: "https://basegraph.app" }],
	creator: "Basegraph",
	publisher: "Basegraph",
	robots: {
		index: true,
		follow: true,
		googleBot: {
			index: true,
			follow: true,
			"max-video-preview": -1,
			"max-image-preview": "large",
			"max-snippet": -1,
		},
	},
	openGraph: {
		type: "website",
		locale: "en_US",
		url: "https://basegraph.app",
		title: "Relay by Basegraph - Turn Tickets into Production-Ready Specs",
		description:
			"Turn tickets into production-ready specs. Relay analyzes your tickets, gathers context from your team, and maps your codebase to generate specs that actually ship.",
		siteName: "Relay by Basegraph",
		images: [
			{
				url: "/og-image.png",
				width: 1200,
				height: 630,
				alt: "Relay by Basegraph - AI-Powered Spec Generation",
			},
		],
	},
	twitter: {
		card: "summary_large_image",
		title: "Relay by Basegraph - Turn Tickets into Production-Ready Specs",
		description:
			"Turn tickets into production-ready specs with deterministic code analysis. Stop letting AI agents guess.",
		creator: "@basegraph",
		site: "@basegraph",
		images: ["/og-image.png"],
	},
	icons: {
		icon: [{ url: "/favicon.svg", type: "image/svg+xml" }],
		apple: [{ url: "/apple-touch-icon.png" }],
	},
	manifest: "/site.webmanifest",
	alternates: {
		canonical: "https://basegraph.app",
	},
};

export const viewport: Viewport = {
	width: "device-width",
	initialScale: 1,
	viewportFit: "cover",
};

export default function RootLayout({
	children,
}: Readonly<{
	children: React.ReactNode;
}>) {
	return (
		<html lang="en" suppressHydrationWarning>
			<body className={`font-serif antialiased`}>
				<ThemeProvider
					attribute="class"
					defaultTheme="light"
					enableSystem={false}
					disableTransitionOnChange={false}
				>
					{children}
					<Analytics />
					<Toaster />
				</ThemeProvider>
			</body>
		</html>
	);
}
