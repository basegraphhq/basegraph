import { RELAY_API_URL } from "@/lib/config";

export type User = {
	id: string;
	name: string;
	email: string;
	avatar_url?: string;
};

export type Session = {
	user: User;
};

export async function getSession(): Promise<Session | null> {
	try {
		const res = await fetch("/api/auth/me", {
			credentials: "include",
		});

		if (!res.ok) {
			return null;
		}

		const data = await res.json();
		return { user: data.user };
	} catch {
		return null;
	}
}

export async function signOut(): Promise<void> {
	await fetch("/api/auth/logout", {
		method: "POST",
		credentials: "include",
	});
}

export function getLoginUrl(): string {
	return "/api/auth/login";
}

// Server-side auth utilities
export const SESSION_COOKIE = "relay_session";

export type ValidateResponse = {
	user: {
		id: string;
		name: string;
		email: string;
		avatar_url?: string;
	};
	has_organization: boolean;
	organization_id?: string;
	workspace_id?: string;
};

/**
 * Server-side session validation (used by middleware)
 * Always uses cache: 'no-store' to prevent stale auth data
 */
export async function validateSession(
	sessionId: string,
): Promise<ValidateResponse | null> {
	try {
		const res = await fetch(`${RELAY_API_URL}/auth/validate`, {
			headers: {
				"X-Session-ID": sessionId,
			},
			cache: "no-store", // Critical: Always fresh for auth!
		});

		if (!res.ok) {
			return null;
		}

		return await res.json();
	} catch (error) {
		console.error("Error validating session:", error);
		return null;
	}
}

/**
 * Extract user context from request headers (injected by middleware proxy)
 * Use this in API routes instead of calling validateSession again!
 */
export function getUserFromHeaders(headers: Headers): {
	sessionId: string;
	userId: string;
	userEmail: string;
	organizationId?: string;
	workspaceId?: string;
	hasOrganization: boolean;
} {
	const sessionId = headers.get("x-session-id");
	const userId = headers.get("x-user-id");
	const userEmail = headers.get("x-user-email");

	if (!sessionId || !userId || !userEmail) {
		throw new Error("Missing auth headers - route not protected by middleware");
	}

	return {
		sessionId,
		userId,
		userEmail,
		organizationId: headers.get("x-organization-id") ?? undefined,
		workspaceId: headers.get("x-workspace-id") ?? undefined,
		hasOrganization: headers.get("x-has-organization") === "true",
	};
}
