import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

export async function POST() {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);

		const refreshResponse = await fetch(
			`${RELAY_API_URL}/api/v1/integrations/gitlab/refresh`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Session-ID": user.sessionId,
				},
				body: JSON.stringify({
					workspace_id: user.workspaceId,
				}),
			},
		);

		if (!refreshResponse.ok) {
			const errorData = await refreshResponse.json().catch(() => ({}));
			return NextResponse.json(
				{ error: errorData.error || "Failed to refresh GitLab integration" },
				{ status: refreshResponse.status },
			);
		}

		const data = await refreshResponse.json();
		return NextResponse.json(data);
	} catch (error) {
		console.error("Error refreshing GitLab integration:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
