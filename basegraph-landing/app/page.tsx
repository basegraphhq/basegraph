export default function Home() {
  return (
    <main className="relative min-h-screen bg-background text-foreground overflow-hidden">
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-0 left-1/4 w-96 h-96 bg-muted rounded-full opacity-5 blur-3xl"></div>
        <div className="absolute bottom-1/3 right-1/4 w-80 h-80 bg-muted rounded-full opacity-5 blur-3xl"></div>
      </div>

      <div className="relative z-10 max-w-2xl mx-auto px-6 py-20 md:py-32">
        <div className="mb-16 md:mb-24">
          <h1 className="text-5xl md:text-6xl font-serif font-bold">Basegraph</h1>
        </div>

        {/* The Problem */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">The Problem</h2>
          <p className="text-lg leading-relaxed mb-4">
            Today's AI coding agents waste time and tokens reading tens or hundreds of files, guessing at each step when given a complex task.
          </p>
          <p className="text-lg leading-relaxed">
            Agentic workflows are exploding, but every one of them hits the same wall: <strong>context</strong>.
          </p>
        </section>

        {/* The Solution */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Our Solution</h2>
          <p className="text-lg leading-relaxed mb-4">
            We built <strong>Codegraph</strong>, a high-performance code-context engine that performs compiler-accurate code lookup and relationship mapping—calls, imports, implementations—in milliseconds.
          </p>
          <p className="text-lg leading-relaxed">
            It fetches only the exact context an agent needs, removing the guesswork and inefficiency of semantic search.
          </p>
        </section>

        {/* Philosophy */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Our Philosophy</h2>
          <p className="text-lg leading-relaxed mb-4">
            The software industry is racing toward a future of fully autonomous AI agents writing thousands of lines of
            code in isolation. We believe this overshoots both current AI capabilities and what developers actually
            need.
          </p>
          <p className="text-lg leading-relaxed mb-4">
            We're building for the present reality, not a distant fantasy.{" "}
            <strong>
              We believe developers should remain in the loop—understanding, learning from, and approving every change.
            </strong>{" "}
            AI should augment human capability, not replace human judgment.
          </p>
          <p className="text-lg leading-relaxed mb-4">
            Our approach centers on manageable collaboration: working in chunks developers can hold in their heads,
            having AI explain its reasoning, requiring it to prove correctness by referencing documentation, and
            encouraging it to ask questions rather than make assumptions. We want developers who use Basegraph to become{" "}
            <em>better programmers</em>, not just passive consumers of machine-generated code.
          </p>
          <p className="text-lg leading-relaxed">
            This philosophy isn't just about developer experience—it's about software quality and security. We believe
            the alternative path leads to mountains of unmaintained code slop, increased vulnerabilities, and systems no
            one truly understands. That's not the future we're building toward.
          </p>
        </section>

        {/* Relay */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Relay</h2>
          <p className="text-lg leading-relaxed mb-4">
            Our first product, <strong>Relay</strong>, is a CLI-based AI agent powered by Codegraph.
          </p>
          <p className="text-lg leading-relaxed mb-4">
            Proven to be <strong>13x faster</strong> and <strong>14x cheaper</strong> compared to leading coding agents at planning complex tasks, with zero hallucinations.
          </p>
          <p className="text-lg leading-relaxed">
            Our native code review bot catches issues before they reach pull requests, but it doesn't just analyze
            syntax—it understands the full context through our compiler backend Codegraph Engine.
            We also capture Jira and Linear issue descriptions, team discussions,
            session goals, todos, and learnings from past decisions, ensuring AI recommendations align with your actual
            intent and requirements.
          </p>
        </section>

        {/* Vision */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Our Vision</h2>
          <p className="text-lg leading-relaxed mb-4">
            We envision a world where AI amplifies developer capabilities without diminishing their agency or
            understanding. Where context isn't lost between tools and tickets. Where code quality improves through
            intelligent collaboration, not blind automation. And where every development team has an AI partner that
            enhances their skills rather than replacing them.
          </p>
          <p className="text-lg leading-relaxed mb-8">
            Basegraph is building the future of coding—one where humans and AI work in true partnership, producing
            better code and better developers.
          </p>
          <p className="text-base text-muted-foreground italic border-t border-border pt-8">
            For developers who think deeply. For teams that build intentionally. For organizations that value both
            velocity and quality.
          </p>
        </section>
      </div>
    </main>
  )
}

