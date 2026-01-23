# Stupid Simple Pastebin

Stupid Simple Pastebin is a web service that makes it easy to paste and share text with other people online. 

- The configuration is environment variables only.
- Each paste is stored as a separate object in S3.
- Meta data about each paste is also stored in S3.
- The name of each paste is the SHA256 checksum of the content.
- The name of echo paste is used in the URL.
- Each paste has a configurable time to live. The time to live is set when
  the paste is created.
- Each paste can be deleted manually from the page of the paste.
- There is a background task that will traverse the pastes at certain intervals
  to remove pastes that have exceeded their time to live.

The front page shows a large text box with syntax highlighting enabled and line
numbers on the left side. Towards the bottom of the page, there is a drop down
that makes it possible to select the time to live when saving a new paste.
