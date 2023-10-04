module.exports = {
	rules: {
		'body-leading-blank': [2, 'always'],
		'body-case': [0, 'never'],
		'footer-leading-blank': [1, 'always'],
		'footer-max-line-length': [2, 'always', 72],
		'header-max-length': [2, 'always', 100],
		'subject-case': [
			2,
			'never',
			['sentence-case', 'start-case', 'pascal-case', 'upper-case'],
		],
		'subject-empty': [2, 'never'],
		'subject-full-stop': [2, 'never', '.'],
		'type-case': [2, 'always', 'lower-case'],
		'type-empty': [2, 'never'],
		'type-enum': [
			2,
			'always',
			[
				'build',
				'chore',
				'ci',
				'docs',
				'feat',
				'fix',
				'perf',
				'refactor',
				'revert',
				'test',
			],
		],
	},
};
