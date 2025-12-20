import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

const SESSION_COOKIE = "relay_session";
const ONBOARDING_COOKIE = "relay_onboarding_complete";

export async function POST() {
	const cookieStore = await cookies();
	const sessionId = cookieStore.get(SESSION_COOKIE)?.value;

	if (sessionId) {
		try {
			await fetch(`${RELAY_API_URL}/auth/logout-session`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ session_id: sessionId }),
			});
		} catch (error) {
			console.error("Error logging out from Relay:", error);
		}
	}

	cookieStore.delete(SESSION_COOKIE);
	cookieStore.delete(ONBOARDING_COOKIE);

	return NextResponse.json({ message: "logged out" });
}
