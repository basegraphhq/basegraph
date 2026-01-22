import { type NextRequest, NextResponse } from "next/server";
import { getClientIP, rateLimit } from "@/lib/rate-limit";

const RATE_LIMIT_MAX_REQUESTS = 5;
const RATE_LIMIT_WINDOW_MS = 15 * 60 * 1000; // 15 minutes

const corsHeaders = {
	"Access-Control-Allow-Origin": "*",
	"Access-Control-Allow-Methods": "POST, OPTIONS",
	"Access-Control-Allow-Headers": "Content-Type",
};

export async function OPTIONS() {
	return new NextResponse(null, {
		status: 200,
		headers: corsHeaders,
	});
}

export async function POST(request: NextRequest) {
	const clientIP = getClientIP(request);
	const rateLimitResult = rateLimit(
		clientIP,
		RATE_LIMIT_MAX_REQUESTS,
		RATE_LIMIT_WINDOW_MS,
	);

	if (!rateLimitResult.success) {
		const retryAfter = Math.ceil((rateLimitResult.reset - Date.now()) / 1000);
		return NextResponse.json(
			{
				error: "Too many requests. Please try again later.",
				retryAfter,
			},
			{
				status: 429,
				headers: {
					"Retry-After": retryAfter.toString(),
					"X-RateLimit-Limit": rateLimitResult.limit.toString(),
					"X-RateLimit-Remaining": rateLimitResult.remaining.toString(),
					"X-RateLimit-Reset": new Date(rateLimitResult.reset).toISOString(),
					...corsHeaders,
				},
			},
		);
	}

	try {
		const { name, email, company, issueTracker, codeHost, message } = await request.json();

		if (!name || !name.trim()) {
			return NextResponse.json(
				{ error: "Name is required" },
				{ status: 400, headers: corsHeaders },
			);
		}

		if (!email || !email.includes("@")) {
			return NextResponse.json(
				{ error: "Invalid email address" },
				{ status: 400, headers: corsHeaders },
			);
		}

		if (!company || !company.trim()) {
			return NextResponse.json(
				{ error: "Company is required" },
				{ status: 400, headers: corsHeaders },
			);
		}

		if (!issueTracker || !issueTracker.trim()) {
			return NextResponse.json(
				{ error: "Issue tracker is required" },
				{ status: 400, headers: corsHeaders },
			);
		}

		if (!codeHost || !codeHost.trim()) {
			return NextResponse.json(
				{ error: "Code host is required" },
				{ status: 400, headers: corsHeaders },
			);
		}

		const airtableApiKey = process.env.AIRTABLE_API_KEY;
		const airtableBaseId = process.env.AIRTABLE_BASE_ID;
		const airtableTableName = process.env.AIRTABLE_TABLE_NAME || "Waitlist";

		if (!airtableApiKey || !airtableBaseId) {
			console.error("Missing Airtable configuration");
			return NextResponse.json(
				{ error: "Server configuration error" },
				{ status: 500, headers: corsHeaders },
			);
		}

		const response = await fetch(
			`https://api.airtable.com/v0/${airtableBaseId}/${encodeURIComponent(airtableTableName)}`,
			{
				method: "POST",
				headers: {
					Authorization: `Bearer ${airtableApiKey}`,
					"Content-Type": "application/json",
				},
				body: JSON.stringify({
					fields: {
						Name: name.trim(),
						Email: email.trim(),
						Company: company.trim(),
						"Issue Tracker": issueTracker.trim(),
						"Code Host": codeHost.trim(),
						Message: message?.trim() || "",
						"Submitted At": new Date().toISOString(),
					},
				}),
			},
		);

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({}));
			console.error("Airtable API error:", errorData);

			if (response.status === 422) {
				return NextResponse.json(
					{ error: "This email has already submitted a request" },
					{ status: 422, headers: corsHeaders },
				);
			}

			return NextResponse.json(
				{ error: "Failed to submit request" },
				{ status: response.status, headers: corsHeaders },
			);
		}

		const data = await response.json();

		return NextResponse.json(
			{ success: true, id: data.id },
			{
				status: 201,
				headers: {
					"X-RateLimit-Limit": rateLimitResult.limit.toString(),
					"X-RateLimit-Remaining": rateLimitResult.remaining.toString(),
					"X-RateLimit-Reset": new Date(rateLimitResult.reset).toISOString(),
					...corsHeaders,
				},
			},
		);
	} catch (error) {
		console.error("Error submitting contact request:", error);
		return NextResponse.json(
			{ error: "Internal server error" },
			{ status: 500, headers: corsHeaders },
		);
	}
}
