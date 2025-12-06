/**
 * Simple in-memory rate limiter
 * For production with multiple instances, consider using Redis-based rate limiting
 */

interface RateLimitEntry {
  count: number
  resetTime: number
}

const rateLimitStore = new Map<string, RateLimitEntry>()

// Clean up expired entries every 5 minutes
setInterval(() => {
  const now = Date.now()
  for (const [key, entry] of rateLimitStore.entries()) {
    if (now > entry.resetTime) {
      rateLimitStore.delete(key)
    }
  }
}, 5 * 60 * 1000)

export interface RateLimitResult {
  success: boolean
  limit: number
  remaining: number
  reset: number
}

/**
 * Rate limit checker
 * @param identifier - Unique identifier (e.g., IP address)
 * @param maxRequests - Maximum number of requests allowed
 * @param windowMs - Time window in milliseconds
 * @returns Rate limit result with success status and metadata
 */
export function rateLimit(
  identifier: string,
  maxRequests: number = 5,
  windowMs: number = 15 * 60 * 1000 // 15 minutes default
): RateLimitResult {
  const now = Date.now()
  const entry = rateLimitStore.get(identifier)

  if (!entry || now > entry.resetTime) {
    // Create new entry or reset expired one
    const newEntry: RateLimitEntry = {
      count: 1,
      resetTime: now + windowMs,
    }
    rateLimitStore.set(identifier, newEntry)
    return {
      success: true,
      limit: maxRequests,
      remaining: maxRequests - 1,
      reset: newEntry.resetTime,
    }
  }

  // Entry exists and is still valid
  if (entry.count >= maxRequests) {
    return {
      success: false,
      limit: maxRequests,
      remaining: 0,
      reset: entry.resetTime,
    }
  }

  // Increment count
  entry.count++
  rateLimitStore.set(identifier, entry)

  return {
    success: true,
    limit: maxRequests,
    remaining: maxRequests - entry.count,
    reset: entry.resetTime,
  }
}

/**
 * Get client IP address from Next.js request
 */
export function getClientIP(request: Request): string {
  // Check various headers that might contain the real IP
  const forwarded = request.headers.get('x-forwarded-for')
  const realIP = request.headers.get('x-real-ip')
  const cfConnectingIP = request.headers.get('cf-connecting-ip') // Cloudflare

  if (forwarded) {
    // x-forwarded-for can contain multiple IPs, take the first one
    return forwarded.split(',')[0].trim()
  }

  if (realIP) {
    return realIP
  }

  if (cfConnectingIP) {
    return cfConnectingIP
  }

  // Fallback (won't work in serverless, but good for development)
  return 'unknown'
}

