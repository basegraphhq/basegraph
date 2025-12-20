"use client";

import { useEffect, useState } from "react";
import { getSession, type Session } from "@/lib/auth";

type UseSessionReturn = {
	data: Session | null;
	status: "loading" | "authenticated" | "unauthenticated";
	isPending: boolean;
};

export function useSession(): UseSessionReturn {
	const [session, setSession] = useState<Session | null>(null);
	const [status, setStatus] = useState<
		"loading" | "authenticated" | "unauthenticated"
	>("loading");

	useEffect(() => {
		getSession().then((data) => {
			setSession(data);
			setStatus(data ? "authenticated" : "unauthenticated");
		});
	}, []);

	return {
		data: session,
		status,
		isPending: status === "loading",
	};
}
