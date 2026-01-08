"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useSession } from "@/hooks/use-session";

export function AuthRedirect() {
	const { data: session, isPending } = useSession();
	const router = useRouter();

	useEffect(() => {
		if (!isPending && session) {
			const target = session.has_organization
				? "/dashboard"
				: "/dashboard/onboarding";
			router.push(target);
		}
	}, [isPending, session, router]);

	return null;
}
