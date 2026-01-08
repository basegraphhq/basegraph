import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

export async function POST(req: Request) {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);
		const { name, slug } = await req.json();

		if (!name) {
			return NextResponse.json(
				{ error: "Organization name is required" },
				{ status: 400 },
			);
		}

		const createOrgResponse = await fetch(
			`${RELAY_API_URL}/api/v1/organizations`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Session-ID": user.sessionId,
				},
				body: JSON.stringify({
					name,
					slug,
					admin_user_id: user.userId,
				}),
			},
		);

		if (!createOrgResponse.ok) {
			const errorText = await createOrgResponse.text();
			return NextResponse.json(
				{ error: "Failed to create organization", details: errorText },
				{ status: createOrgResponse.status },
			);
		}

		const orgData = await createOrgResponse.json();
		return NextResponse.json(orgData);
	} catch (error) {
		console.error("Error creating organization:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
