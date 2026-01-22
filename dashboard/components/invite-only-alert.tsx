"use client";

import { useSearchParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
	AlertDialog,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

export function InviteOnlyAlert() {
	const searchParams = useSearchParams();
	const router = useRouter();
	const [open, setOpen] = useState(false);

	useEffect(() => {
		if (searchParams.get("error") === "invite_only") {
			setOpen(true);
		}
	}, [searchParams]);

	const handleClose = () => {
		setOpen(false);
		router.replace("/");
	};

	return (
		<AlertDialog open={open} onOpenChange={setOpen}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>Invite Only</AlertDialogTitle>
					<AlertDialogDescription>
						Relay is currently invite-only. If you'd like early access, please get in touch with us.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<Button onClick={handleClose}>Got it</Button>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
