import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { SESSION_COOKIE, ValidationStatus, validateSession } from "@/lib/auth";

/**
 * Unified proxy - Handles both page routing and API auth
 */
export async function proxy(request: NextRequest) {
	const { pathname } = request.nextUrl;

	// Route to appropriate handler
	if (pathname.startsWith("/api/")) {
		return handleApiAuth(request);
	} else {
		return handlePageAuth(request);
	}
}

/**
 * API Route Authentication Proxy
 * Validates session and injects user context into headers
 */
async function handleApiAuth(request: NextRequest) {
	const sessionId = request.cookies.get(SESSION_COOKIE)?.value;

	if (!sessionId) {
		return NextResponse.json({ error: "Not authenticated" }, { status: 401 });
	}

	// Validate session once (not in every route!)
	const result = await validateSession(sessionId);

	if (result.status === ValidationStatus.Invalid) {
		return NextResponse.json({ error: "Session invalid" }, { status: 401 });
	}

	if (result.status === ValidationStatus.Error) {
		return NextResponse.json(
			{ error: "Service temporarily unavailable" },
			{ status: 503 },
		);
	}

	const validateData = result.data;

	// Check if route requires organization setup
	const requiresOrganization =
		request.nextUrl.pathname.startsWith("/api/integrations");

	if (
		requiresOrganization &&
		(!validateData.organization_id || !validateData.workspace_id)
	) {
		return NextResponse.json(
			{ error: "Organization setup required" },
			{ status: 400 },
		);
	}

	// Attach user context to request headers
	// Route handlers can read these instead of re-validating!
	const requestHeaders = new Headers(request.headers);
	requestHeaders.set("x-session-id", sessionId);
	requestHeaders.set("x-user-id", validateData.user.id);
	requestHeaders.set("x-user-email", validateData.user.email);
	requestHeaders.set(
		"x-has-organization",
		String(validateData.has_organization),
	);

	if (validateData.organization_id) {
		requestHeaders.set("x-organization-id", validateData.organization_id);
	}
	if (validateData.workspace_id) {
		requestHeaders.set("x-workspace-id", validateData.workspace_id);
	}

	return NextResponse.next({
		request: {
			headers: requestHeaders,
		},
	});
}

/**
 * Page Routing Proxy
 * Handles dashboard redirects based on auth and onboarding state
 */
async function handlePageAuth(request: NextRequest) {
	const { pathname } = request.nextUrl;
	const sessionId = request.cookies.get(SESSION_COOKIE)?.value;

	// Redirect to login if not authenticated
	if (!sessionId) {
		if (pathname.startsWith("/dashboard")) {
			return NextResponse.redirect(new URL("/", request.url));
		}
		return NextResponse.next();
	}

	// Check organization status
	const result = await validateSession(sessionId);

	if (result.status === ValidationStatus.Invalid) {
		// Session truly invalid - clear cookie and redirect to login
		const res = pathname.startsWith("/dashboard")
			? NextResponse.redirect(new URL("/", request.url))
			: NextResponse.next();
		res.cookies.delete(SESSION_COOKIE);
		return res;
	}

	if (result.status === ValidationStatus.Error) {
		// Transient error - show error page without logging user out
		return new NextResponse("Service temporarily unavailable", {
			status: 503,
			headers: { "Content-Type": "text/plain" },
		});
	}

	const hasOrganization = result.data.has_organization;
	let response: NextResponse | undefined;

	// Redirect authenticated users from root
	if (pathname === "/") {
		const target = hasOrganization ? "/dashboard" : "/dashboard/onboarding";
		response = NextResponse.redirect(new URL(target, request.url));
	}

	// Redirect users without org to onboarding
	if (
		!response &&
		pathname.startsWith("/dashboard") &&
		pathname !== "/dashboard/onboarding"
	) {
		if (!hasOrganization) {
			response = NextResponse.redirect(
				new URL("/dashboard/onboarding", request.url),
			);
		}
	}

	// Redirect users with org away from onboarding
	if (!response && pathname === "/dashboard/onboarding") {
		if (hasOrganization) {
			response = NextResponse.redirect(new URL("/dashboard", request.url));
		}
	}

	return response ?? NextResponse.next();
}

export const config = {
	matcher: [
		// Page routes
		"/",
		"/dashboard/:path*",
		// API routes (excluding public auth endpoints)
		"/api/organization/:path*",
		"/api/integrations/:path*",
		"/api/agent-status/:path*",
	],
};
