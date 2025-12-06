import { betterAuth } from "better-auth"

export const auth = betterAuth({
  baseURL: process.env.NEXT_PUBLIC_APP_URL,
  socialProviders: {
    // google: {
    //     clientId: process.env.GOOGLE_CLIENT_ID as string,
    //     clientSecret: process.env.GOOGLE_CLIENT_SECRET as string,
    // },
    // github: {
    //     clientId: process.env.GITHUB_CLIENT_ID as string,
    //     clientSecret: process.env.GITHUB_CLIENT_SECRET as string,
    //     callbackURL: "/api/auth/callback/github",
    // },
    gitlab: { 
      clientId: process.env.GITLAB_CLIENT_ID as string, 
      clientSecret: process.env.GITLAB_CLIENT_SECRET as string, 
      issuer: process.env.GITLAB_ISSUER as string, 
      scope: ['read_user']
  }, 
    // linear: {
    //   clientId: process.env.LINEAR_CLIENT_ID as string,
    //   clientSecret: process.env.LINEAR_CLIENT_SECRET as string,
    // },
  },
})