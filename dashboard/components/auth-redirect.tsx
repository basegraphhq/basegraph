"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useSession } from "@/hooks/use-session";

export function AuthRedirect() {
	const { data: session, isPending } = useSession();
	const router = useRouter();

	useEffect(() => {
		if (!isPending && session) {
			router.push("/dashboard");
		}
	}, [isPending, session, router]);

	return null;
}
