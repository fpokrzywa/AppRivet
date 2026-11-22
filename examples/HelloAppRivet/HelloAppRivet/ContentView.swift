import SwiftUI

enum Greeting {
    static func message(for name: String) -> String {
        "Hello, \(name)!"
    }
}

struct ContentView: View {
    var body: some View {
        VStack(spacing: 16) {
            Text(Greeting.message(for: "AppRivet"))
                .font(.title)
            Text("Your first iOS app is ready to build.")
        }
        .padding()
    }
}
