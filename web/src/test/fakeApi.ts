// Components call the API through openapi-fetch, which captures `fetch` when the client
// module loads. The setup file installs this delegating fetch before any test imports it,
// and each test says how the API answers.
type Answer = (request: Request) => Response | Promise<Response>;

let answer: Answer | undefined;

export function installFakeApi() {
  globalThis.fetch = async (input, init) => {
    if (!answer) {
      throw new Error("This test made an API request but did not set answerWith().");
    }
    return answer(input instanceof Request ? input : new Request(input, init));
  };
}

export function answerWith(next: Answer) {
  answer = next;
}

export function resetFakeApi() {
  answer = undefined;
}

export function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
