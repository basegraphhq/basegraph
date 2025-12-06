# Google Authentication Setup

This project uses [better-auth](https://www.better-auth.com/) for Google OAuth authentication.

## Environment Variables

Add the following environment variables to your `.env.local` file:

```env
# App Configuration
NEXT_PUBLIC_APP_URL=http://localhost:3000

# Google OAuth Configuration
GOOGLE_CLIENT_ID=your_google_client_id_here
GOOGLE_CLIENT_SECRET=your_google_client_secret_here
```

## Getting Google OAuth Credentials

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google+ API
4. Go to "Credentials" in the left sidebar
5. Click "Create Credentials" → "OAuth client ID"
6. Choose "Web application"
7. Add authorized redirect URIs:
   - For development: `http://localhost:3000/api/auth/callback/google`
   - For production: `https://yourdomain.com/api/auth/callback/google`
8. Copy the Client ID and Client Secret to your `.env.local` file

## How It Works

### Components

- **LoginButton** (`components/login-button.tsx`): A client component that triggers the Google OAuth flow
- **Auth Client** (`lib/auth-client.ts`): Client-side auth utilities using better-auth/react
- **Auth Server** (`lib/auth.ts`): Server-side auth configuration
- **API Route** (`app/api/auth/[...all]/route.ts`): Handles all auth-related API requests

### Flow

1. User clicks "Sign in with Google" button in the top right
2. Client calls `signIn.social({ provider: "google" })`
3. User is redirected to Google's OAuth consent screen
4. After approval, Google redirects back to `/api/auth/callback/google`
5. better-auth handles the callback and creates a session
6. User is redirected back to the homepage

## Usage

The login button is positioned in the top right corner of the page, next to the theme toggle.

To check if a user is logged in, use the `useSession` hook:

```tsx
import { useSession } from "@/lib/auth-client"

function MyComponent() {
  const { data: session } = useSession()
  
  if (session) {
    // User is logged in
    console.log(session.user)
  }
  
  return <div>...</div>
}
```

## Production Deployment

When deploying to production:

1. Update `NEXT_PUBLIC_APP_URL` to your production domain
2. Add the production callback URL to your Google OAuth credentials
3. Ensure environment variables are set in your hosting platform

