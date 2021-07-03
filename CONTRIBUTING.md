# How to Contribute

This document outlines some of the conventions on development workflow.

## Getting Started

- Fork the repository on GitHub
- Read the [README](README.md) for build and test instructions
- Read the [STYLEGUIDE](STYLEGUIDE.md) for code conventions

## Contribution Flow

This is a rough outline of what a contributor's workflow looks like:

- Create a topic branch from where you want to base your work (usually master).
- Make commits of logical units.
- Make sure your commit messages are in the proper format (see below).
- Push your changes to a topic branch in your fork of the repository.
- Make sure the tests and liting pass, and add any new tests as appropriate.
- Submit a pull request to the original repository.

> 🎯 Tip: make sure to install the githook using the command: `make githooks`

## Format of the Commit Message

We follow a rough convention for commit messages that is designed to answer two
questions: what changed and why. The subject line should feature the what and
the body of the commit should describe the why.

```
Add the gitgooks command

This add githooks to the project, in order to run tests
and liting and prevent bad commits.
```

> 🚨 Warning: It requires  `golanglint-ci >= 1.39`

## Pull Request Formats

Pull Requests should use the template provided, and
follow the template instructions. For those that implement new
enchancements or backporting must have on its own title the reference
to the Bugzilla bug.


**For new enchancements:**

```
Bug 1940432: Gahter datahubs.installers.datahub.sap.com resources from SAP clusters
```

**For backports:**

```
[release-4.6] Bug 1942907: Gather datahubs.installers.datahub.sap.com resources from SAP clusters
```

## Backporting

Branches for previous releases follow the format `release-X.Y`, for example,
`release-4.1`. Typically, bugs are fixed in the master branch first then
backported to the appropriate release branches. Fixes backported to previous
releases should have a Bugzilla bug for each version fixed.
