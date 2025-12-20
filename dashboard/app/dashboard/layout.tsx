import { TopNavBar } from "@/components/top-nav-bar";

export default function DashboardLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<div className="min-h-svh bg-background flex flex-col">
			<TopNavBar />
			<main className="flex-1 overflow-auto">{children}</main>
		</div>
	);
}
