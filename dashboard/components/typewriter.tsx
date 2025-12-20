"use client";

import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";

export interface TypewriterProps {
	/** Array of messages to type out sequentially */
	messages: string[];
	/** Milliseconds per character (default: 15) */
	typingSpeed?: number;
	/** Milliseconds to pause between messages (default: 200) */
	delayBetweenMessages?: number;
	/** Additional className for the container */
	className?: string;
	/** Additional className for each line */
	lineClassName?: string;
	/** Callback when typing is complete */
	onComplete?: () => void;
}

/**
 * Typewriter effect component that types out messages character by character.
 */
export function Typewriter({
	messages,
	typingSpeed = 7,
	delayBetweenMessages = 200,
	className,
	lineClassName = "text-muted-foreground",
	onComplete,
}: TypewriterProps) {
	const [displayedLines, setDisplayedLines] = useState<string[]>([]);
	const [currentMessageIndex, setCurrentMessageIndex] = useState(0);
	const [currentText, setCurrentText] = useState("");

	useEffect(() => {
		if (currentMessageIndex >= messages.length) {
			if (onComplete) {
				onComplete();
			}
			return;
		}

		const currentMessage = messages[currentMessageIndex];

		if (currentText.length < currentMessage.length) {
			// Still typing current message
			const timeout = setTimeout(() => {
				setCurrentText(currentMessage.slice(0, currentText.length + 1));
			}, typingSpeed);
			return () => clearTimeout(timeout);
		} else {
			// Finished typing current message, move to next
			const timeout = setTimeout(() => {
				setDisplayedLines((prev) => [...prev, currentMessage]);
				setCurrentText("");
				setCurrentMessageIndex((prev) => prev + 1);
			}, delayBetweenMessages);
			return () => clearTimeout(timeout);
		}
	}, [
		currentText,
		currentMessageIndex,
		messages,
		typingSpeed,
		delayBetweenMessages,
		onComplete,
	]);

	return (
		<div className={cn("space-y-1", className)}>
			{displayedLines.map((line, index) => (
				<p key={index} className={lineClassName}>
					{line}
				</p>
			))}
			{currentMessageIndex < messages.length && (
				<p className={lineClassName}>{currentText}</p>
			)}
		</div>
	);
}
