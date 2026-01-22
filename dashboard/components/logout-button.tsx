"use client";

import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { signOut } from "@/lib/auth";

type LogoutButtonProps = {
	/** If true, also signs out from WorkOS (clears WorkOS session) */
	fullLogout?: boolean;
};

export function LogoutButton({ fullLogout = true }: LogoutButtonProps) {
	const router = useRouter();

	const handleLogout = async () => {
		try {
			const workosLogoutUrl = await signOut({
				fullLogout,
				returnTo: window.location.origin,
			});

			// If full logout and we have a WorkOS logout URL, redirect to it
			// WorkOS will redirect back to our returnTo URL after clearing their session
			if (fullLogout && workosLogoutUrl) {
				window.location.href = workosLogoutUrl;
				return;
			}

			// Otherwise, just redirect to home
			router.push("/");
		} catch (error) {
			console.error("Logout failed:", error);
		}
	};

	return (
		<Button
			onClick={handleLogout}
			variant="outline"
			className="font-serif text-sm border-none shadow-none underline"
		>
			Sign out
		</Button>
	);
}
