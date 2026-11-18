package com.starriver.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import mobilebind.Mobilebind

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val version = Mobilebind.version()
        val echo = Mobilebind.echo("ping-中文-🌌")
        val profile = Mobilebind.deviceProfile()
        setContent {
            Column(Modifier.padding(24.dp)) {
                Text("VERSION: $version")
                Text("ECHO: $echo")
                Text("PROFILE: $profile")
            }
        }
    }
}
