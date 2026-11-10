import 'package:flutter/material.dart';

import 'device_list_page.dart';

class FarmApp extends StatelessWidget {
  const FarmApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Farm',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF5267E8)),
        useMaterial3: true,
      ),
      home: const DeviceListPage(),
    );
  }
}
