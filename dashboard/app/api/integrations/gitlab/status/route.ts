import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

export async function GET(_req: Request) {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);
		const statusResponse = await fetch(
			`${RELAY_API_URL}/api/v1/integrations/gitlab/status?workspace_id=${user.workspaceId}`,
			{
				headers: {
					"X-Session-ID": user.sessionId,
				},
				cache: "no-store",
			},
		);

		if (!statusResponse.ok) {
			const errorData = await statusResponse.json().catch(() => ({}));
			return NextResponse.json(
				{ error: errorData.error || "Failed to fetch GitLab status" },
				{ status: statusResponse.status },
			);
		}

		const data = await statusResponse.json();
		return NextResponse.json(data);
	} catch (error) {
		console.error("Error fetching GitLab status:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
