import { type NextRequest, NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

export async function GET(request: NextRequest) {
	const token = request.nextUrl.searchParams.get("token");

	if (!token) {
		return NextResponse.json(
			{ error: "Token is required", code: "missing_token" },
			{ status: 400 },
		);
	}

	try {
		const res = await fetch(
			`${RELAY_API_URL}/invites/validate?token=${encodeURIComponent(token)}`,
		);

		const data = await res.json();

		if (!res.ok) {
			return NextResponse.json(
				{ error: data.error, code: data.code },
				{ status: res.status },
			);
		}

		return NextResponse.json(data);
	} catch (error) {
		console.error("Error validating invite:", error);
		return NextResponse.json(
			{ error: "Failed to validate invitation", code: "server_error" },
			{ status: 500 },
		);
	}
}
