import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

export const runtime = "nodejs";

export async function GET() {
	const headersList = await headers();
	let user: ReturnType<typeof getUserFromHeaders>;
	try {
		user = getUserFromHeaders(headersList);
	} catch (error) {
		return NextResponse.json(
			{ error: error instanceof Error ? error.message : "Unauthorized" },
			{ status: 401 },
		);
	}

	if (!user.organizationId || !user.workspaceId) {
		return NextResponse.json(
			{ error: "Organization setup required" },
			{ status: 400 },
		);
	}

	const url = new URL(
		`${RELAY_API_URL}/api/v1/agent-status/orgs/${user.organizationId}/workspaces/${user.workspaceId}/stream`,
	);
	url.searchParams.set("last_id", "0-0");

	const response = await fetch(url.toString(), {
		headers: {
			"X-Session-ID": user.sessionId,
			Accept: "text/event-stream",
		},
	});

	if (!response.ok || !response.body) {
		return NextResponse.json(
			{ error: "Failed to connect to status stream" },
			{ status: response.status || 502 },
		);
	}

	return new Response(response.body, {
		status: response.status,
		headers: {
			"Content-Type": "text/event-stream",
			"Cache-Control": "no-cache",
			Connection: "keep-alive",
		},
	});
}
