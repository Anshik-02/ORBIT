import Navbar from "@/components/landing/Navbar";
import Hero from "@/components/landing/Hero";
import ArchitectureOverview from "@/components/landing/ArchitectureOverview";
import WritePipeline from "@/components/landing/WritePipeline";
import TechStack from "@/components/landing/TechStack";
import Footer from "@/components/landing/Footer";

export default function Home() {
  return (
    <main className="min-h-screen bg-[#F4F7FC] text-[#17243A] flex flex-col selection:bg-[#0891D5]/20 selection:text-[#0891D5]">
      {/* Section A: Navigation Bar */}
      <Navbar />

      {/* Section B: Hero Section with 3D Cluster Visualization */}
      <Hero />

      {/* Section C: Architecture Overview with 5 Interactive Cards */}
      <ArchitectureOverview />

      {/* Section D: Write Request Pipeline Animation with Failure Simulation Toggle */}
      <WritePipeline />

      {/* Section E: Technology Stack */}
      <TechStack />

      {/* Section F: Final CTA & Footer */}
      <Footer />
    </main>
  );
}
