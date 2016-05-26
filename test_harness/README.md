SDK test harness
===========================
The json files in testdata/ exist here so we can test sdk-specific scenarios that are not
covered by our integration-harness which can test multiple SDKs at once.
An example of this is prerequisite cycle detection which happens both on the server-side
and the client side. We double-check cycle detection on the client for extra safety.

