"use client";

import { ChevronsUpDown, LogOut, Moon, Plug, Sun } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import * as React from "react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
	Sidebar,
	SidebarContent,
	SidebarFooter,
	SidebarHeader,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
} from "@/components/ui/sidebar";
import { useSession } from "@/hooks/use-session";
import { signOut } from "@/lib/auth";

export function AppSidebar() {
	const { data: session } = useSession();
	const router = useRouter();
	const { theme, setTheme } = useTheme();
	const [mounted, setMounted] = React.useState(false);

	React.useEffect(() => {
		setMounted(true);
	}, []);

	const handleSignOut = async () => {
		await signOut();
		router.push("/");
	};

	return (
		<Sidebar collapsible="none" className="h-svh">
			<SidebarHeader className="page-header px-4 justify-center ">
				<Link href="/" className="flex  items-center gap-2 ">
					<span className="h4">Basegraph</span>
				</Link>
			</SidebarHeader>

			<SidebarContent className="px-2 pt-1">
				<SidebarMenu>
					<SidebarMenuItem>
						<SidebarMenuButton asChild>
							<Link href="/dashboard/">
								<Plug className="size-4" />
								<span>Integrations</span>
							</Link>
						</SidebarMenuButton>
					</SidebarMenuItem>
				</SidebarMenu>
			</SidebarContent>

			<SidebarFooter className="px-2 py-2">
				<SidebarMenu>
					<SidebarMenuItem>
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<SidebarMenuButton
									size="lg"
									className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground focus-visible:ring-0"
								>
									<Avatar className="h-8 w-8 rounded-lg">
										<AvatarImage
											src={session?.user?.avatar_url || undefined}
											alt={session?.user?.name || ""}
										/>
										<AvatarFallback className="rounded-lg">
											{session?.user?.name?.slice(0, 2).toUpperCase() || "U"}
										</AvatarFallback>
									</Avatar>
									<div className="grid flex-1 text-left leading-tight">
										<span className="truncate text-sm font-semibold text-sidebar-foreground">
											{session?.user?.name || "User"}
										</span>
										<span className="truncate text-xs text-muted-foreground">
											{session?.user?.email || "email@example.com"}
										</span>
									</div>
									<ChevronsUpDown className="ml-auto size-4" />
								</SidebarMenuButton>
							</DropdownMenuTrigger>
							<DropdownMenuContent
								className="w-[--radix-dropdown-menu-trigger-width] min-w-56 rounded-lg"
								side="top"
								align="start"
								sideOffset={4}
							>
								<DropdownMenuItem
									onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
									className="cursor-pointer"
								>
									{mounted && theme === "dark" ? (
										<Sun className="size-4" />
									) : (
										<Moon className="size-4" />
									)}
									<span>
										{mounted && theme === "dark" ? "Light mode" : "Dark mode"}
									</span>
								</DropdownMenuItem>
								<DropdownMenuSeparator />
								<DropdownMenuItem
									onClick={handleSignOut}
									className="cursor-pointer"
								>
									<LogOut className="size-4" />
									Log out
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</SidebarMenuItem>
				</SidebarMenu>
			</SidebarFooter>
		</Sidebar>
	);
}
