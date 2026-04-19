agent: coder "software developer"
  model: claude-3-5-sonnet-20241022
  role: developer
  system_prompt: You are a helpful software developer. When asked questions, provide clear and concise answers about code.

agent: reviewer "code reviewer" 
  model: claude-3-5-sonnet-20241022
  role: reviewer
  system_prompt: You are a code reviewer. Review code for quality, security, and best practices.

flow: coder -> reviewer