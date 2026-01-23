import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

export async function GET() {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);
		const response = await fetch(
			`${RELAY_API_URL}/api/v1/integrations/gitlab/repos/enabled?workspace_id=${user.workspaceId}`,
			{
				headers: {
					"X-Session-ID": user.sessionId,
				},
				cache: "no-store",
			},
		);

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({}));
			return NextResponse.json(
				{ error: errorData.error || "Failed to fetch enabled repositories" },
				{ status: response.status },
			);
		}

		const data = await response.json();
		return NextResponse.json(data);
	} catch (error) {
		console.error("Error fetching enabled GitLab repositories:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
