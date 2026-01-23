import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { getUserFromHeaders } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

type SetupRequest = {
	instance_url: string;
	token: string;
};

type SetupResponse = {
	integration_id: string;
	is_new_integration: boolean;
	projects: Array<{
		id: number;
		name: string;
		path_with_namespace: string;
		web_url: string;
		description?: string;
		default_branch?: string;
	}>;
	webhooks_created: number;
	repositories_added: number;
	errors?: string[];
};

export async function POST(req: Request) {
	try {
		const headersList = await headers();
		const user = getUserFromHeaders(headersList);

		const body: SetupRequest = await req.json();

		if (!body.instance_url || !body.token) {
			return NextResponse.json(
				{ error: "instance_url and token are required" },
				{ status: 400 },
			);
		}

		const setupResponse = await fetch(
			`${RELAY_API_URL}/api/v1/integrations/gitlab/setup`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Session-ID": user.sessionId,
				},
				body: JSON.stringify({
					instance_url: body.instance_url,
					token: body.token,
					workspace_id: user.workspaceId,
					organization_id: user.organizationId,
					setup_by_user_id: user.userId,
				}),
			},
		);

		if (!setupResponse.ok) {
			const errorData = await setupResponse.json().catch(() => ({}));
			console.error("GitLab setup failed:", errorData);
			return NextResponse.json(
				{ error: errorData.error || "Failed to setup GitLab integration" },
				{ status: setupResponse.status },
			);
		}

		const setupData: SetupResponse = await setupResponse.json();
		return NextResponse.json(setupData);
	} catch (error) {
		console.error("Error setting up GitLab integration:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500 },
		);
	}
}
