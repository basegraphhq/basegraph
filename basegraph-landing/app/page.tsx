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

        {/* Mission */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Our Mission</h2>
          <p className="text-lg leading-relaxed mb-4">
            At Basegraph, we're reimagining the future of software development by putting humans back at the center of
            AI-assisted coding. We believe that the most powerful code isn't written by AI alone—it's crafted through
            intelligent collaboration between human creativity and machine precision.
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

        {/* What We're Building */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">What We're Building</h2>
          <p className="text-lg leading-relaxed mb-4">
            We're developing a Human-Centered AI Code Editor that embodies this philosophy. Unlike tools that generate
            massive changesets and disappear for 20 minutes, Basegraph works alongside you in real-time, keeping you
            aware and in control.
          </p>
          <p className="text-lg leading-relaxed">
            Our native code review bot catches issues before they reach pull requests, but it doesn't just analyze
            syntax—it understands the full context. It captures Jira and Linear issue descriptions, team discussions,
            session goals, todos, and learnings from past decisions, ensuring AI recommendations align with your actual
            intent and requirements.
          </p>
        </section>

        {/* Technical Approach */}
        <section className="mb-16 md:mb-24">
          <h2 className="text-3xl md:text-4xl font-serif font-bold mb-6">Our Technical Approach</h2>
          <p className="text-lg leading-relaxed">
            We're pioneering specialized Small Language Models (SLMs) trained specifically for individual codebases.
            While the industry chases general-purpose solutions, we believe the answer lies in AI that deeply
            understands your unique architecture, patterns, and business logic—delivering accuracy and relevance that
            outstrips one-size-fits-all large language models.
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
