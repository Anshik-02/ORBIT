import Navbar from "@/components/landing/Navbar";
import Hero from "@/components/landing/Hero";
import ArchitectureOverview from "@/components/landing/ArchitectureOverview";
import WritePipeline from "@/components/landing/WritePipeline";
import TechStack from "@/components/landing/TechStack";
import Footer from "@/components/landing/Footer";

export default function Home() {
  return (
    <main className="min-h-screen bg-[#030712] text-slate-100 flex flex-col selection:bg-sky-500/30 selection:text-sky-200">
      {/* Section A: Navigation Bar */}
      <Navbar />

      {/* Section B: Hero Section with 3D Cluster Visualization */}
      <Hero />

      {/* Section C: Architecture Overview with 5 Interactive Cards */}
      <ArchitectureOverview />

      {/* Section D: Write Request Pipeline Animation */}
      <WritePipeline />

      {/* Section E: Technology Stack */}
      <TechStack />

      {/* Section F: Final CTA & Footer */}
      <Footer />
    </main>
  );
}
