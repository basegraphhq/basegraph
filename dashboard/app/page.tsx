import Link from "next/link";
import { Suspense } from "react";
import { AuthRedirect } from "@/components/auth-redirect";
import { ContactDialog } from "@/components/contact-dialog";
import { InviteOnlyAlert } from "@/components/invite-only-alert";
import { ThemeToggle } from "@/components/theme-toggle";

export default function Home() {
	const jsonLd = {
		"@context": "https://schema.org",
		"@type": "SoftwareApplication",
		name: "Relay by Basegraph",
		applicationCategory: "DeveloperApplication",
		operatingSystem: "Web",
		offers: {
			"@type": "Offer",
			price: "0",
			priceCurrency: "USD",
		},
		description:
			"Turn tickets into production-ready specs. Relay analyzes your tickets, gathers context from your team, and maps your codebase to generate specs that actually ship.",
		url: "https://basegraph.co",
		creator: {
			"@type": "Organization",
			name: "Basegraph",
			url: "https://basegraph.co",
			sameAs: ["https://x.com/basegraph"],
		},
		featureList: [
			"Deterministic code analysis",
			"Linear integration",
			"Automated spec generation",
			"Codebase mapping",
			"Team context gathering",
		],
	};

	return (
		<main className="relative min-h-screen bg-background text-foreground overflow-hidden">
			{/* Redirect signed-in users to dashboard */}
			<AuthRedirect />

			{/* Show invite-only alert when redirected with error=invite_only */}
			<Suspense fallback={null}>
				<InviteOnlyAlert />
			</Suspense>

			{/* Structured Data */}
			<script
				type="application/ld+json"
				dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
			/>

			{/* Theme Toggle */}
			<ThemeToggle />

			{/* Subtle geometric background */}
			<div className="absolute inset-0 pointer-events-none overflow-hidden">
				<div className="absolute top-0 left-1/4 w-[600px] h-[600px] bg-gradient-to-br from-primary/5 to-transparent rounded-full blur-3xl" />
				<div className="absolute bottom-1/4 right-0 w-[500px] h-[500px] bg-gradient-to-tl from-accent/5 to-transparent rounded-full blur-3xl" />
				<div className="absolute top-1/2 left-0 w-[400px] h-[400px] bg-gradient-to-r from-secondary/5 to-transparent rounded-full blur-3xl" />
			</div>

			<div className="relative z-10">
				{/* Hero Section */}
				<section
					className="page-container pt-20 pb-16 md:pt-32 md:pb-24"
					aria-label="Hero"
				>
					<div className="mb-6 cluster">
						{/* <Image src="/logo.svg" alt="Relay logo" width={28} height={28} /> */}
						<span className="text-overline">Relay by Basegraph</span>
					</div>

					<h1 className="text-display mb-8">
						Turn tickets into
						<br />
						<span className="text-accent">production-ready specs</span>
					</h1>

					<p className="text-lead max-w-2xl mb-10">
						Relay analyzes your tickets, gathers context from your team, and
						maps your codebase to generate specs that actually ship. Hand them
						to any developer or AI agent.
					</p>

					<div className="mb-12">
						<ContactDialog />
					</div>

					<div className="cluster text-sm text-muted-foreground">
						<div className="status-dot status-dot-pulse" />
						<span className="text-mono">
							Powered by <strong className="text-foreground">Codegraph</strong>.
							Deterministic Code Context Lookup.
						</span>
					</div>
				</section>

				{/* Philosophy */}
				<section
					className="border-t border-border/50"
					aria-labelledby="philosophy-heading"
				>
					<div className="page-container section-padding">
						<div className="grid md:grid-cols-[1fr,2fr] gap-8 md:gap-16">
							<div>
								<h2 id="philosophy-heading">
									Context Lives in People, Not Just Code.
								</h2>
							</div>
							<div className="stack stack-lg">
								<p className="text-lg text-body-secondary">
									Edge cases don't live in your codebase—they live in your
									team's heads. Business logic nuances. Production gotchas. That
									one integration everyone forgets about.
								</p>
								<p className="text-lg text-body-secondary">
									Relay asks the right questions. It pulls context from the
									people who know your product, then maps that against your
									actual codebase constraints. What you get is a spec that
									accounts for both the business logic edge cases humans catch
									and the architectural limitations code analysis reveals.
								</p>
								<p className="text-lg text-foreground font-medium">
									This is the step most AI tools skip. We built Relay because
									planning is too important to automate away.
								</p>
							</div>
						</div>
					</div>
				</section>

				{/* Why It Works */}
				<section
					className="bg-card/50 border-y border-border/50"
					aria-labelledby="comparison-heading"
				>
					<div className="page-container section-padding">
						<h2 id="comparison-heading" className="mb-16">
							Why Deterministic Beats Semantic
						</h2>

						<div className="grid md:grid-cols-2 gap-12 md:gap-16">
							{/* Problem with current tools */}
							<div>
								<h3 className="mb-4 text-muted-foreground">
									Semantic Search (Cursor, Aider, etc.)
								</h3>
								<ul className="stack text-body-tertiary">
									<li className="flex gap-3">
										<span className="text-accent mt-1">×</span>
										<span>
											Embeddings miss exact import chains and call graphs
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">×</span>
										<span>
											Can't verify if a function actually exists or trace its
											dependencies
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">×</span>
										<span>
											Guesses at architecture based on similarity, not structure
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">×</span>
										<span>
											Reads 50+ files hoping to find the right context
										</span>
									</li>
								</ul>
							</div>

							{/* Codegraph approach */}
							<div>
								<h3 className="mb-4">Codegraph (Compiler-Based)</h3>
								<ul className="stack">
									<li className="flex gap-3">
										<span className="text-accent mt-1">→</span>
										<span>
											Traces exact import paths and function call chains
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">→</span>
										<span>
											Verifies every symbol exists and maps its relationships
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">→</span>
										<span>
											Understands actual architecture from parse trees, not
											vibes
										</span>
									</li>
									<li className="flex gap-3">
										<span className="text-accent mt-1">→</span>
										<span>Fetches only the 3-5 files that actually matter</span>
									</li>
								</ul>
							</div>
						</div>

						<div className="mt-16 p-6 card-subtle border border-border rounded-lg">
							<p className="text-lg text-body-secondary">
								<strong className="text-foreground">Result:</strong> Your AI
								agent gets a spec with verified imports, actual function
								signatures, and real architectural constraints—not hallucinated
								ones. No more "this function doesn't exist" or "wrong number of
								parameters" errors after 20 minutes of generation.
							</p>
						</div>
					</div>
				</section>

				{/* Integration */}
				<section
					className="border-b border-border/50"
					aria-labelledby="integration-heading"
				>
					<div className="page-container section-padding">
						<h2 id="integration-heading" className="mb-16">
							Ships Where You Work
						</h2>

						<div className="grid md:grid-cols-2 gap-12">
							<div className="stack">
								<h3>Onboarding in Minutes</h3>
								<p className="text-lg text-body-secondary">
									Connect your Linear workspace. Point to your repo. Done.
								</p>
								<p className="text-body-tertiary">
									No training data to upload. No context windows to configure.
									Relay immediately understands your codebase through static
									analysis.
								</p>
							</div>

							<div className="stack">
								<h3>No New Dashboard</h3>
								<p className="text-lg text-body-secondary">
									Relay lives in Linear threads. Specs appear as comments. Your
									workflow stays intact.
								</p>
								<p className="text-body-tertiary">
									Everything happens where you already work. No
									context-switching to yet another tool.
								</p>
							</div>
						</div>

						<div className="mt-12 cluster">
							<span className="text-caption text-muted-foreground">
								GitHub, Jira, Linear coming Q1 2026
							</span>
						</div>
					</div>
				</section>

				{/* Closing */}
				<section
					className="border-b border-border/50"
					aria-label="Call to action"
				>
					<div className="page-container py-16 md:py-20 text-center">
						<p className="text-lead max-w-3xl mx-auto mb-8">
							Stop letting your AI agents guess. Give them specs that actually
							work.
						</p>
						<ContactDialog />
					</div>
				</section>

				{/* Early Access Note */}
				<section
					className="border-b border-border/50"
					aria-labelledby="early-access-heading"
				>
					<div className="page-container py-12 md:py-16">
						<div className="stack stack-lg">
							<h2 id="early-access-heading" className="h4">
								Early Access
							</h2>
							<p className="text-body-secondary">
								Relay is in private beta. Code indexing currently happens on our
								servers. Self-hosted deployments are on the roadmap for teams
								with strict data residency requirements.
							</p>
							<p className="text-body-secondary">
								We're working closely with early adopters to shape the product.
								We'd love to hear your feedback and thoughts.
							</p>
							<div className="pt-2">
								<Link
									href="mailto:nithinsj@basegraph.co"
									className="inline-flex items-center gap-2 text-mono text-sm text-link"
								>
									Reach out → nithinsj@basegraph.co
								</Link>
								<Link
									href="mailto:nithinsudarsan@basegraph.co"
									className="inline-flex items-center gap-2 text-mono text-sm text-link"
								>
									, nithinsudarsan@basegraph.co
								</Link>
							</div>
							<Link
								href="https://discord.gg/gD3dq2as"
								className="inline-flex items-center gap-2 text-mono text-sm text-link"
							>
								Join our Discord
							</Link>
						</div>
					</div>
				</section>

				{/* Footer */}
				<footer className="page-container py-12 md:py-16">
					<div className="flex flex-col md:flex-row md:items-center justify-between gap-6">
						<Link
							href="https://x.com/basegraph"
							className="text-caption text-muted-foreground hover:text-foreground transition-colors"
						>
							X
						</Link>
						<Link
							href="https://discord.gg/gD3dq2as"
							className="text-caption text-muted-foreground hover:text-foreground transition-colors"
						>
							Discord
						</Link>
						<p className="text-caption text-muted-foreground">
							© 2025 Basegraph.
						</p>
					</div>
				</footer>
			</div>
		</main>
	);
}
