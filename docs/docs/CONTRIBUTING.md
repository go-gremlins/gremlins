# Contributing to Gremlins

First, thanks for you wanting to contribute, the Gremlins project welcomes contributors!

## What can I contribute?

### Report bugs

Bug reports are always welcome. Before submitting one, please verify that a similar bug hasn't been already reported. If
it already exists, consider to comment there instead of opening a new one.

#### To submit a good bug report

Use the appropriate template to submit bugs.

- Use a clear and descriptive title.
- Describe each step to reproduce using as much detail as you can.
- Describe the behaviour you observed after following the steps.
- Explain why it is different from the behaviour you would expect.

### Suggest enhancements

Before making a feature request/enhancement request, please verify that there isn't already a request for the same or
very similar feature. If a similar enhancement request already exists, you can expand on it via comments.
There is a specific issue type for feature requests.

### Send pull requests

Pull request are welcome, but it's not guaranteed they will be accepted. We are quite strict on code quality, style and
code metrics, bear with us if we ask you to make changes before accepting your PR.

## Becoming a contributor

Gremlins is fully developed on GitHub and the best way to contribute is by forking the repository and, once you complete
your work, opening a _pull request_.

### Before contributing

All contributions are welcome, but, before submitting any significant change, it is better to coordinate with the
Gremlins' team before starting the work. It is a good idea to start at
the [issue tracker](https://github.com/go-gremlins/gremlins/issues) and file a new issue or claim an existing one.

### Open an issue

Apart from trivial changes, every contribution to the Gremlins should be linked to an issue. Feel free to propose a
change and expose your plans, so that everyone can contribute in its validation, and the chances of your _pull request_
being accepted will increase.

### Submit a contribution

Gremlins is released with semantic versioning and
follows [GitHub flow](https://docs.github.com/en/get-started/quickstart/github-flow), with the only difference that we
open release branches when there is a _fix version_ to release.

When you open a _pull request_, a series of automatic checks kicks off. You can verify if your change makes those checks
fail and adjust the code accordingly.

At this point, a member of the Gremlins team will review your code, possibly will
discuss with you to understand it better, maybe ask for some changes and so on. Please expect that this process will be
more thorough if you are a first time contributor: we need to know each other.

Feel free to make more than one commit in your _pull request_, but bear in mind that once it has been properly reviewed
and accepted, all the commits will be squashed into one before merging. This way we can maintain a linear commit
history.

Before a release, the code base will be frozen and no _pull request_ will be accepted until the release is done, with
the only exception of bug fixes. If you send a _pull request_ during a code freeze, you will have to wait a little more
before seeing it merged.

### Commit messages

Commit messages follow a convention. Here is an example:

```
area: do a specific thing

Expand on what and how it is done, possibly spanning multiple lines and
being descriptive.

Fixes #123
```

#### First line

This is a short one line summary of what has been done in the commit, prefixed by the package affected (ex. `mutator`
, `docs`, etc.). The commit message should be written as it is answering the question "This commit changes Gremlins to
..."

The first line is separated from the rest by a blank line.

#### Message body

The message body expands on the first line, adding details in a descriptive way. Try to use correct grammar and
punctuation, and don't use Markdown, HTML or other markup languages.

#### Reference

If the commit is related to an issue (most of the time it does), you can add a reference in the
format `KEYWORD #ISSUE_NUMBER`. This
helps [GitHub link](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue#linking-a-pull-request-to-an-issue-using-a-keyword)
the commit to the appropriate issue and update its status.

The recognized keywords are:

- close
- closes
- closed
- fix
- fixes
- fixed
- resolve
- resolves
- resolved

Sometimes the team members as well forget to respect all of these rules, but we do our best to be consistent. If you
forget or make mistakes, we will help during the review process.