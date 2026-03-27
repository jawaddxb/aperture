module.exports = {
  apps: [{
    name: "aperture-server",
    script: "./aperture-server",
    cwd: "/Users/jawad/.openclaw/workspace-builder/aperture",
    env: {
      APERTURE_LLM_API_KEY: process.env.OPENROUTER_API_KEY,
      APERTURE_LLM_BASE_URL: "https://openrouter.ai/api/v1",
      APERTURE_LLM_PROVIDER: "openai",
      APERTURE_LLM_MODEL: "gpt-4o-mini",
    }
  }]
};
