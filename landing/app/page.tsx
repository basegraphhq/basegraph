import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { ArrowRight, Code, GitBranch, Zap, Brain, Shield, Users, CheckCircle, Star, Github, Twitter, Linkedin } from 'lucide-react'

export default function LandingPage() {
  return (
    <div className="min-h-screen bg-white relative">
      {/* Abstract Pattern Background */}
      <div 
        className="absolute inset-0 opacity-5"
        style={{
          backgroundImage: 'url(/images/abstract-pattern.png)',
          backgroundSize: 'cover',
          backgroundPosition: 'center',
          backgroundRepeat: 'no-repeat'
        }}
      />
      
      {/* Header */}
      <header className="border-b border-gray-200 bg-white/90 backdrop-blur-sm sticky top-0 z-50 relative">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <div className="w-8 h-8 bg-gray-900 rounded-lg flex items-center justify-center">
              <Code className="w-5 h-5 text-white" />
            </div>
            <span className="text-xl font-bold text-gray-900">Basegraph</span>
          </div>
          <nav className="hidden md:flex items-center space-x-8">
            <a href="#features" className="text-gray-600 hover:text-gray-900 transition-colors">Features</a>
            <a href="#benefits" className="text-gray-600 hover:text-gray-900 transition-colors">Benefits</a>
            <a href="#pricing" className="text-gray-600 hover:text-gray-900 transition-colors">Pricing</a>
            <Button variant="outline" className="mr-2 border-gray-300 text-gray-700 hover:bg-gray-50">Sign In</Button>
            <Button className="bg-gray-900 hover:bg-gray-800 text-white">Get Started</Button>
          </nav>
        </div>
      </header>

      {/* Hero Section */}
      <section className="py-20 px-4 relative">
        <div className="container mx-auto text-center max-w-4xl">
          <Badge variant="secondary" className="mb-4 bg-gray-100 text-gray-700 border-gray-200">
            <Zap className="w-3 h-3 mr-1" />
            AI-Powered Code Intelligence
          </Badge>
          <h1 className="text-5xl md:text-6xl font-bold text-gray-900 mb-6 leading-tight">
            Supercharge Your Code with{" "}
            <span className="text-gray-700 underline decoration-gray-300 decoration-4 underline-offset-8">
              AI Assistants
            </span>
          </h1>
          <p className="text-xl text-gray-600 mb-8 leading-relaxed">
            Experience next-generation code reviews and real-time IDE assistance powered by our advanced codegraph engine. 
            Get unparalleled code context and intelligent suggestions that understand your entire codebase.
          </p>
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Button size="lg" className="text-lg px-8 py-3 bg-gray-900 hover:bg-gray-800 text-white">
              Start Free Trial
              <ArrowRight className="ml-2 w-5 h-5" />
            </Button>
            <Button variant="outline" size="lg" className="text-lg px-8 py-3 border-gray-300 text-gray-700 hover:bg-gray-50">
              Watch Demo
            </Button>
          </div>
          <div className="mt-12 flex items-center justify-center space-x-8 text-sm text-gray-500">
            <div className="flex items-center">
              <CheckCircle className="w-4 h-4 mr-2 text-gray-600" />
              Free 14-day trial
            </div>
            <div className="flex items-center">
              <CheckCircle className="w-4 h-4 mr-2 text-gray-600" />
              No credit card required
            </div>
            <div className="flex items-center">
              <CheckCircle className="w-4 h-4 mr-2 text-gray-600" />
              Setup in minutes
            </div>
          </div>
        </div>
      </section>

      {/* Features Section */}
      <section id="features" className="py-20 px-4 bg-gray-50 relative">
        <div className="container mx-auto">
          <div className="text-center mb-16">
            <h2 className="text-4xl font-bold text-gray-900 mb-4">
              Powerful Features for Modern Development
            </h2>
            <p className="text-xl text-gray-600 max-w-2xl mx-auto">
              Our AI assistants integrate seamlessly into your workflow, providing intelligent insights at every step.
            </p>
          </div>
          
          <div className="grid md:grid-cols-3 gap-8">
            <Card className="border-2 border-gray-200 hover:border-gray-300 transition-colors bg-white">
              <CardHeader>
                <div className="w-12 h-12 bg-gray-100 rounded-lg flex items-center justify-center mb-4">
                  <GitBranch className="w-6 h-6 text-gray-700" />
                </div>
                <CardTitle className="text-gray-900">AI Code Reviews</CardTitle>
                <CardDescription className="text-gray-600">
                  Automated code reviews that understand context, catch bugs, and suggest improvements before they reach production.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2 text-sm text-gray-600">
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Security vulnerability detection
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Performance optimization suggestions
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Code quality improvements
                  </li>
                </ul>
              </CardContent>
            </Card>

            <Card className="border-2 border-gray-200 hover:border-gray-300 transition-colors bg-white">
              <CardHeader>
                <div className="w-12 h-12 bg-gray-100 rounded-lg flex items-center justify-center mb-4">
                  <Zap className="w-6 h-6 text-gray-700" />
                </div>
                <CardTitle className="text-gray-900">Real-time IDE Assistance</CardTitle>
                <CardDescription className="text-gray-600">
                  Get instant code suggestions, completions, and explanations directly in your favorite IDE.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2 text-sm text-gray-600">
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Intelligent code completion
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Context-aware suggestions
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Multi-language support
                  </li>
                </ul>
              </CardContent>
            </Card>

            <Card className="border-2 border-gray-200 hover:border-gray-300 transition-colors bg-white">
              <CardHeader>
                <div className="w-12 h-12 bg-gray-100 rounded-lg flex items-center justify-center mb-4">
                  <Brain className="w-6 h-6 text-gray-700" />
                </div>
                <CardTitle className="text-gray-900">Codegraph Engine</CardTitle>
                <CardDescription className="text-gray-600">
                  Our proprietary engine analyzes your entire codebase to provide unmatched context and understanding.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2 text-sm text-gray-600">
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Deep codebase analysis
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Cross-file dependencies
                  </li>
                  <li className="flex items-center">
                    <CheckCircle className="w-4 h-4 mr-2 text-gray-700" />
                    Architecture insights
                  </li>
                </ul>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      {/* Benefits Section */}
      <section id="benefits" className="py-20 px-4 relative">
        <div className="container mx-auto">
          <div className="grid lg:grid-cols-2 gap-12 items-center">
            <div>
              <h2 className="text-4xl font-bold text-gray-900 mb-6">
                Why Developers Choose Basegraph
              </h2>
              <div className="space-y-6">
                <div className="flex items-start space-x-4">
                  <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center flex-shrink-0">
                    <Shield className="w-4 h-4 text-gray-700" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900 mb-2">Enhanced Code Quality</h3>
                    <p className="text-gray-600">Catch bugs early and maintain consistent code standards across your entire team.</p>
                  </div>
                </div>
                
                <div className="flex items-start space-x-4">
                  <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center flex-shrink-0">
                    <Users className="w-4 h-4 text-gray-700" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900 mb-2">Team Productivity</h3>
                    <p className="text-gray-600">Reduce review time by 60% and help junior developers learn faster with AI guidance.</p>
                  </div>
                </div>
                
                <div className="flex items-start space-x-4">
                  <div className="w-8 h-8 bg-gray-100 rounded-lg flex items-center justify-center flex-shrink-0">
                    <Zap className="w-4 h-4 text-gray-700" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900 mb-2">Faster Development</h3>
                    <p className="text-gray-600">Ship features 40% faster with intelligent code suggestions and automated reviews.</p>
                  </div>
                </div>
              </div>
            </div>
            
            <div className="bg-gray-100 rounded-2xl p-8">
              <div className="bg-white rounded-lg p-6 shadow-lg border border-gray-200">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center space-x-2">
                    <div className="w-3 h-3 bg-red-500 rounded-full"></div>
                    <div className="w-3 h-3 bg-yellow-500 rounded-full"></div>
                    <div className="w-3 h-3 bg-green-500 rounded-full"></div>
                  </div>
                  <Badge variant="secondary" className="bg-gray-100 text-gray-700">AI Review</Badge>
                </div>
                <div className="space-y-3 text-sm">
                  <div className="text-gray-600">{'// Basegraph AI detected potential issues:'}</div>
                  <div className="text-red-600">⚠️ Memory leak in event listener</div>
                  <div className="text-yellow-600">💡 Consider using useMemo for optimization</div>
                  <div className="text-green-600">✅ Security: Input validation looks good</div>
                  <div className="text-gray-700">🔧 Suggested refactor: Extract custom hook</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Social Proof */}
      <section className="py-16 px-4 bg-gray-50 relative">
        <div className="container mx-auto text-center">
          <h2 className="text-3xl font-bold text-gray-900 mb-8">
            Trusted by Development Teams Worldwide
          </h2>
          <div className="flex items-center justify-center space-x-8 mb-8">
            <div className="flex items-center space-x-1">
              {[...Array(5)].map((_, i) => (
                <Star key={i} className="w-5 h-5 fill-yellow-400 text-yellow-400" />
              ))}
              <span className="ml-2 text-gray-600">4.9/5 from 2,000+ developers</span>
            </div>
          </div>
          <div className="grid md:grid-cols-3 gap-8 max-w-4xl mx-auto">
            <Card className="bg-white border-gray-200">
              <CardContent className="p-6">
                <p className="text-gray-600 mb-4">
                  "Basegraph has transformed our code review process. We catch bugs 3x faster and our junior developers are learning at an incredible pace."
                </p>
                <div className="flex items-center space-x-3">
                  <div className="w-10 h-10 bg-gray-800 rounded-full flex items-center justify-center text-white font-semibold">
                    JS
                  </div>
                  <div>
                    <div className="font-semibold text-gray-900">Jane Smith</div>
                    <div className="text-sm text-gray-500">Senior Engineer, TechCorp</div>
                  </div>
                </div>
              </CardContent>
            </Card>
            
            <Card className="bg-white border-gray-200">
              <CardContent className="p-6">
                <p className="text-gray-600 mb-4">
                  "The codegraph engine is incredible. It understands our complex architecture better than most of our team members!"
                </p>
                <div className="flex items-center space-x-3">
                  <div className="w-10 h-10 bg-gray-700 rounded-full flex items-center justify-center text-white font-semibold">
                    MJ
                  </div>
                  <div>
                    <div className="font-semibold text-gray-900">Mike Johnson</div>
                    <div className="text-sm text-gray-500">CTO, StartupXYZ</div>
                  </div>
                </div>
              </CardContent>
            </Card>
            
            <Card className="bg-white border-gray-200">
              <CardContent className="p-6">
                <p className="text-gray-600 mb-4">
                  "We've reduced our deployment bugs by 80% since implementing Basegraph. It's like having a senior developer reviewing every line of code."
                </p>
                <div className="flex items-center space-x-3">
                  <div className="w-10 h-10 bg-gray-600 rounded-full flex items-center justify-center text-white font-semibold">
                    AL
                  </div>
                  <div>
                    <div className="font-semibold text-gray-900">Alex Lee</div>
                    <div className="text-sm text-gray-500">Lead Developer, InnovateLab</div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      {/* CTA Section */}
      <section className="py-20 px-4 bg-gray-900 relative">
        <div className="container mx-auto text-center">
          <h2 className="text-4xl font-bold text-white mb-6">
            Ready to Transform Your Development Workflow?
          </h2>
          <p className="text-xl text-gray-300 mb-8 max-w-2xl mx-auto">
            Join thousands of developers who are already building better code with Basegraph's AI assistants.
          </p>
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Button size="lg" className="text-lg px-8 py-3 bg-white text-gray-900 hover:bg-gray-100">
              Start Free Trial
              <ArrowRight className="ml-2 w-5 h-5" />
            </Button>
            <Button size="lg" variant="outline" className="text-lg px-8 py-3 border-gray-600 text-gray-300 hover:bg-gray-800 hover:text-white">
              Schedule Demo
            </Button>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="bg-gray-800 text-white py-12 px-4 relative">
        <div className="container mx-auto">
          <div className="grid md:grid-cols-4 gap-8">
            <div>
              <div className="flex items-center space-x-2 mb-4">
                <div className="w-8 h-8 bg-white rounded-lg flex items-center justify-center">
                  <Code className="w-5 h-5 text-gray-800" />
                </div>
                <span className="text-xl font-bold">Basegraph</span>
              </div>
              <p className="text-gray-400 mb-4">
                AI-powered code intelligence for modern development teams.
              </p>
              <div className="flex space-x-4">
                <Github className="w-5 h-5 text-gray-400 hover:text-white cursor-pointer" />
                <Twitter className="w-5 h-5 text-gray-400 hover:text-white cursor-pointer" />
                <Linkedin className="w-5 h-5 text-gray-400 hover:text-white cursor-pointer" />
              </div>
            </div>
            
            <div>
              <h3 className="font-semibold mb-4">Product</h3>
              <ul className="space-y-2 text-gray-400">
                <li><a href="#" className="hover:text-white">Features</a></li>
                <li><a href="#" className="hover:text-white">Pricing</a></li>
                <li><a href="#" className="hover:text-white">Integrations</a></li>
                <li><a href="#" className="hover:text-white">API</a></li>
              </ul>
            </div>
            
            <div>
              <h3 className="font-semibold mb-4">Company</h3>
              <ul className="space-y-2 text-gray-400">
                <li><a href="#" className="hover:text-white">About</a></li>
                <li><a href="#" className="hover:text-white">Blog</a></li>
                <li><a href="#" className="hover:text-white">Careers</a></li>
                <li><a href="#" className="hover:text-white">Contact</a></li>
              </ul>
            </div>
            
            <div>
              <h3 className="font-semibold mb-4">Support</h3>
              <ul className="space-y-2 text-gray-400">
                <li><a href="#" className="hover:text-white">Documentation</a></li>
                <li><a href="#" className="hover:text-white">Help Center</a></li>
                <li><a href="#" className="hover:text-white">Community</a></li>
                <li><a href="#" className="hover:text-white">Status</a></li>
              </ul>
            </div>
          </div>
          
          <div className="border-t border-gray-700 mt-8 pt-8 text-center text-gray-400">
            <p>&copy; 2024 Basegraph. All rights reserved.</p>
          </div>
        </div>
      </footer>
    </div>
  )
}
