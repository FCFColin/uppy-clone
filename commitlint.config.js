// Enterprise rationale: Conventional Commits enable:
// 1. Auto-generated CHANGELOGs
// 2. Semantic versioning automation
// 3. Git history readability
// Used by Angular, React, and most major open-source projects.

module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [
      2,
      'always',
      [
        'feat',     // New feature
        'fix',      // Bug fix
        'docs',     // Documentation
        'style',    // Code style (formatting, missing semi-colons, etc)
        'refactor', // Code refactoring
        'perf',     // Performance improvement
        'test',     // Adding tests
        'chore',    // Build process or auxiliary tool changes
        'ci',       // CI/CD changes
        'revert',   // Revert commit
        'security', // Security fix
      ],
    ],
    'scope-case': [2, 'always', 'lower-case'],
    'subject-max-length': [2, 'always', 72],
  },
};
