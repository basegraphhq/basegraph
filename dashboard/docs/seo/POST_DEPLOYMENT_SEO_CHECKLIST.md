# Post-Deployment SEO Checklist

## Immediate Actions (Do These First!)

### 1. Create Required Images
- [ ] **OG Image** (`/public/og-image.png`)
  - Size: 1200 x 630 pixels
  - Include: Relay logo, tagline, key visual
  - See `/public/OG_IMAGE_SPECS.txt` for details

- [ ] **Apple Touch Icon** (`/public/apple-touch-icon.png`)
  - Size: 180 x 180 pixels
  - Square logo/icon

### 2. Google Search Console Setup
- [ ] Go to https://search.google.com/search-console
- [ ] Add property: https://basegraph.app
- [ ] Verify ownership (DNS or HTML file method)
- [ ] Submit sitemap: https://basegraph.app/sitemap.xml
- [ ] Request indexing for homepage

### 3. Test Social Media Cards
- [ ] **Facebook OG Debugger**
  - URL: https://developers.facebook.com/tools/debug/
  - Enter: https://basegraph.app
  - Click "Scrape Again" if needed

- [ ] **Twitter Card Validator**
  - URL: https://cards-dev.twitter.com/validator
  - Enter: https://basegraph.app
  - Verify card displays correctly

- [ ] **LinkedIn Post Inspector**
  - URL: https://www.linkedin.com/post-inspector/
  - Enter: https://basegraph.app
  - Check preview

### 4. Validate Structured Data
- [ ] **Schema Markup Validator**
  - URL: https://validator.schema.org/
  - Enter: https://basegraph.app
  - Verify no errors in JSON-LD

- [ ] **Google Rich Results Test**
  - URL: https://search.google.com/test/rich-results
  - Enter: https://basegraph.app
  - Check for eligible rich results

### 5. Performance Testing
- [ ] **Google PageSpeed Insights**
  - URL: https://pagespeed.web.dev/
  - Test: https://basegraph.app
  - Aim for 90+ scores
  - Check Core Web Vitals

- [ ] **Mobile-Friendly Test**
  - URL: https://search.google.com/test/mobile-friendly
  - Verify mobile optimization

### 6. Analytics Setup
- [ ] Verify Vercel Analytics is working
- [ ] Consider adding Google Analytics 4
- [ ] Set up conversion tracking for waitlist signups

## Week 1 Actions

### Monitor & Fix
- [ ] Check Google Search Console for crawl errors
- [ ] Monitor indexing status
- [ ] Fix any issues reported

### Content
- [ ] Share on social media (Twitter, LinkedIn)
- [ ] Post on Product Hunt (optional)
- [ ] Share in relevant communities (HN, Reddit r/webdev, Dev.to)

## Ongoing SEO Tasks

### Monthly
- [ ] Review Google Search Console performance
- [ ] Check for broken links
- [ ] Monitor page speed
- [ ] Review and update meta descriptions if needed
- [ ] Check competitor rankings

### Quarterly
- [ ] Audit content for freshness
- [ ] Update structured data if product changes
- [ ] Review and improve keyword strategy
- [ ] Build quality backlinks

### Content Ideas for Future
- [ ] Add blog section for content marketing
- [ ] Create case studies/testimonials
- [ ] Write technical documentation
- [ ] Add FAQ section
- [ ] Create video content

## Tools & Resources

### Free SEO Tools
- Google Search Console: https://search.google.com/search-console
- Google Analytics: https://analytics.google.com/
- PageSpeed Insights: https://pagespeed.web.dev/
- Mobile-Friendly Test: https://search.google.com/test/mobile-friendly
- Schema Validator: https://validator.schema.org/

### Monitoring
- Uptime monitoring: https://uptimerobot.com/
- Vercel Analytics (already installed)

### Learning Resources
- Google SEO Starter Guide: https://developers.google.com/search/docs/fundamentals/seo-starter-guide
- Next.js SEO: https://nextjs.org/learn/seo/introduction-to-seo

## Quick Wins

### Easy Improvements
- [ ] Add alt text to any missing images
- [ ] Ensure all links have descriptive text
- [ ] Add more internal links between pages (when you add more pages)
- [ ] Keep content fresh and updated

### Link Building
- [ ] List on relevant directories (ProductHunt, BetaList, etc.)
- [ ] Reach out to developer blogs for features
- [ ] Engage with Linear and AI tool communities
- [ ] Guest post on developer blogs

## Success Metrics

Track these over time:
- Organic search traffic
- Keyword rankings
- Click-through rate (CTR) in search results
- Bounce rate
- Time on page
- Waitlist conversion rate

## Notes
- SEO is a long-term game - results take 3-6 months
- Focus on quality content and user experience
- Build genuine backlinks from relevant sources
- Keep technical SEO maintained (this is done!)
- Monitor and iterate based on data

---

**Current Status**: ✅ Technical SEO fully optimized
**Next Step**: Create OG image and submit to Google Search Console

