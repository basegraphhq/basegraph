import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

type EnableRequest = {
	project_ids: number[];
};

export async function POST(req: Request) {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);

		const body: EnableRequest = await req.json();

		if (!body.project_ids || body.project_ids.length === 0) {
			return NextResponse.json(
				{ error: "project_ids are required" },
				{ status: 400 },
			);
		}

		const response = await fetch(
			`${RELAY_API_URL}/api/v1/integrations/gitlab/repos/enable`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Session-ID": user.sessionId,
				},
				body: JSON.stringify({
					workspace_id: user.workspaceId,
					project_ids: body.project_ids,
				}),
			},
		);

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({}));
			return NextResponse.json(
				{ error: errorData.error || "Failed to enable repositories" },
				{ status: response.status },
			);
		}

		const data = await response.json();
		return NextResponse.json(data);
	} catch (error) {
		console.error("Error enabling GitLab repositories:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
