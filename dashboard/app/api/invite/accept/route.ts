import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

const SESSION_COOKIE = "relay_session";

type AcceptRequest = {
	token: string;
};

export async function POST(request: NextRequest) {
	const cookieStore = await cookies();
	const sessionId = cookieStore.get(SESSION_COOKIE)?.value;

	if (!sessionId) {
		return NextResponse.json(
			{ error: "Not authenticated", code: "unauthenticated" },
			{ status: 401 },
		);
	}

	let body: AcceptRequest;
	try {
		body = await request.json();
	} catch {
		return NextResponse.json(
			{ error: "Invalid request body", code: "invalid_request" },
			{ status: 400 },
		);
	}

	if (!body.token) {
		return NextResponse.json(
			{ error: "Token is required", code: "missing_token" },
			{ status: 400 },
		);
	}

	try {
		const res = await fetch(`${RELAY_API_URL}/invites/accept`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				"X-Session-ID": sessionId,
			},
			body: JSON.stringify({ token: body.token }),
		});

		const data = await res.json();

		if (!res.ok) {
			return NextResponse.json(
				{ error: data.error, code: data.code },
				{ status: res.status },
			);
		}

		return NextResponse.json(data);
	} catch (error) {
		console.error("Error accepting invite:", error);
		return NextResponse.json(
			{ error: "Failed to accept invitation", code: "server_error" },
			{ status: 500 },
		);
	}
}
